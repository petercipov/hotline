package export

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// batchSize is how many records one send carries at most.
const batchSize = 512

// queue holds the records for one destination and sends them in batches.
// When it is full, the oldest record is dropped and counted, so a call never
// waits for a destination.
type queue struct {
	// Processor gives the methods a queue has nothing to add to.
	sdklog.Processor

	exporter sdklog.Exporter
	endpoint string
	size     int
	interval time.Duration
	logger   *slog.Logger
	// stats count what the queue did, as the queue's processor and the
	// destination's sender; up is the destination's state.
	stats     sendStats
	processor attribute.Set
	sender    attribute.Set
	up        *atomic.Int64

	mu      sync.Mutex
	records []sdklog.Record
	// crowded counts records dropped from a full queue, failed those lost
	// in a send the destination refused.
	crowded drops
	failed  drops
	lastErr error
	sendMu  sync.Mutex
	full    chan struct{}
	stop    chan struct{}
	stopped chan struct{}
}

func newQueue(exporter sdklog.Exporter, endpoint string, size int, interval time.Duration, logger *slog.Logger) *queue {
	q := &queue{
		Processor: sdklog.NewSimpleProcessor(nil),
		exporter:  exporter,
		endpoint:  endpoint,
		size:      size,
		interval:  interval,
		logger:    logger,
		full:      make(chan struct{}, 1),
		stop:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	go q.run()
	return q
}

// OnEmit queues a record without waiting.
func (q *queue) OnEmit(_ context.Context, r *sdklog.Record) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.records) == q.size {
		q.records = q.records[1:]
		q.crowded.count++
		q.stats.processed.Add(context.Background(), 1,
			metric.WithAttributes(append(q.processor.ToSlice(), semconv.ErrorTypeKey.String("queue_full"))...))
	}
	q.records = append(q.records, r.Clone())
	if len(q.records) >= batchSize {
		select {
		case q.full <- struct{}{}:
		default:
		}
	}
	return nil
}

func (q *queue) run() {
	defer close(q.stopped)
	ticker := time.NewTicker(q.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-q.full:
		case <-q.stop:
			return
		}
		_ = q.send(context.Background(), false)
	}
}

// send sends what is queued, in batches; all of it when flushing, else one
// batch. A batch the destination refuses is lost and counted: the
// destination has had its retries.
func (q *queue) send(ctx context.Context, all bool) error {
	q.sendMu.Lock()
	defer q.sendMu.Unlock()
	defer q.warnDropped(all)
	for {
		q.mu.Lock()
		batch := q.records[:min(batchSize, len(q.records))]
		q.records = q.records[len(batch):]
		q.mu.Unlock()
		if len(batch) == 0 {
			return nil
		}
		q.stats.processed.Add(ctx, int64(len(batch)), metric.WithAttributeSet(q.processor))
		err := q.exporter.Export(ctx, batch)
		measured(ctx, q.stats.logs, len(batch), q.sender, err, q.up)
		if err != nil {
			q.mu.Lock()
			q.failed.count += len(batch)
			q.lastErr = err
			q.mu.Unlock()
		}
		if err != nil || !all {
			return err
		}
	}
}

// drops counts lost records until the next warning about them.
type drops struct {
	count  int
	warned time.Time
}

// due says a warning about the drops is due: at most once a minute, and
// always on the last send.
func (d *drops) due(last bool) bool {
	if d.count == 0 || !last && time.Since(d.warned) < time.Minute {
		return false
	}
	d.warned = time.Now()
	return true
}

// warnDropped writes how many records were lost, for each of the two
// reasons.
func (q *queue) warnDropped(last bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.crowded.due(last) {
		q.logger.Warn("export queue full, dropped the oldest records", "endpoint", q.endpoint, "dropped", q.crowded.count)
		q.crowded.count = 0
	}
	if q.failed.due(last) {
		q.logger.Warn("export failed, dropped the records it held", "endpoint", q.endpoint, "dropped", q.failed.count, "error", q.lastErr.Error())
		q.failed.count = 0
	}
}

// fill is how many records wait, and how many the queue holds at most.
func (q *queue) fill() (int, int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.records), q.size
}

// ForceFlush sends everything queued.
func (q *queue) ForceFlush(ctx context.Context) error {
	return q.send(ctx, true)
}

// Shutdown stops the queue, sends what is left and closes the destination.
func (q *queue) Shutdown(ctx context.Context) error {
	close(q.stop)
	<-q.stopped
	err := q.ForceFlush(ctx)
	return errors.Join(err, q.exporter.Shutdown(ctx))
}
