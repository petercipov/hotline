package export

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	sdklog "go.opentelemetry.io/otel/sdk/log"
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

	mu      sync.Mutex
	records []sdklog.Record
	dropped int
	warned  time.Time
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
		q.dropped++
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
// batch. A batch the destination refuses is lost: the destination has had
// its retries.
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
		if err := q.exporter.Export(ctx, batch); err != nil || !all {
			return err
		}
	}
}

// warnDropped writes how many records were dropped, at most once a minute,
// and always on the last send.
func (q *queue) warnDropped(last bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.dropped == 0 || !last && time.Since(q.warned) < time.Minute {
		return
	}
	q.logger.Warn("export queue full, dropped the oldest records", "endpoint", q.endpoint, "dropped", q.dropped)
	q.dropped = 0
	q.warned = time.Now()
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
