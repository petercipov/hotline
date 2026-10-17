package ddsketch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// LogLineVersion is the format version written into every log line. A reader
// rejects any other version rather than guessing at its meaning.
const LogLineVersion = 1

// ErrInvalidLogLine rejects a log line that does not decode to a consistent
// sketch.
var ErrInvalidLogLine = errors.New("ddsketch: invalid log line")

// logLine is the wire form of a sketch: one JSON object per line, so a sketch
// can be published through an ordinary logging pipeline and folded back in by
// whoever reads the logs.
//
// Buckets are written sparse and sorted by index, so the same state always
// encodes to the same bytes. Min and max are left out of an empty sketch,
// because its min is math.MaxUint64, which most JSON readers cannot hold
// exactly.
type logLine struct {
	Version   int         `json:"v"`
	Range     []uint64    `json:"range"`
	Count     uint64      `json:"count"`
	Sum       uint64      `json:"sum"`
	Min       *uint64     `json:"min,omitempty"`
	Max       *uint64     `json:"max,omitempty"`
	Zeros     uint64      `json:"zeros"`
	Underflow uint64      `json:"underflow"`
	Overflow  uint64      `json:"overflow"`
	Buckets   []logBucket `json:"buckets"`
}

// logBucket is one populated bucket, written as the pair [index, count].
type logBucket struct {
	Index int32
	Count uint64
}

func (b logBucket) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{b.Index, b.Count})
}

func (b *logBucket) UnmarshalJSON(data []byte) error {
	var pair []json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if len(pair) != 2 {
		return fmt.Errorf("%w: bucket must hold [index, count]", ErrInvalidLogLine)
	}
	if err := json.Unmarshal(pair[0], &b.Index); err != nil {
		return err
	}
	return json.Unmarshal(pair[1], &b.Count)
}

// MarshalLogLine encodes the full internal state as a single line of JSON with
// no trailing newline. ParseLogLine restores it bit exactly.
func (s *Sketch) MarshalLogLine() ([]byte, error) {
	line := logLine{
		Version:   LogLineVersion,
		Range:     []uint64{s.rng.MinNS, s.rng.MaxNS},
		Count:     s.count,
		Sum:       s.sum,
		Zeros:     s.zeros,
		Underflow: s.underflowCount,
		Overflow:  s.overflowCount,
		Buckets:   make([]logBucket, 0, len(s.buckets)),
	}
	if s.count > 0 {
		line.Min = &s.min
		line.Max = &s.max
	}
	// Insert and Merge only ever add to a bucket, so none is ever zero
	for _, i := range s.SortedBucketIndexes() {
		line.Buckets = append(line.Buckets, logBucket{Index: i, Count: s.buckets[i]})
	}
	return json.Marshal(line)
}

// ParseLogLine decodes a line written by MarshalLogLine. A trailing newline is
// accepted. Anything that would not decode to a sketch Insert and Merge could
// have produced is rejected with ErrInvalidLogLine.
func ParseLogLine(data []byte) (*Sketch, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var line logLine
	if err := decoder.Decode(&line); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidLogLine, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("%w: trailing data after the sketch", ErrInvalidLogLine)
	}
	if line.Version != LogLineVersion {
		return nil, fmt.Errorf("%w: unsupported version %d", ErrInvalidLogLine, line.Version)
	}
	if len(line.Range) != 2 {
		return nil, fmt.Errorf("%w: range must hold [min, max]", ErrInvalidLogLine)
	}
	rng := Range{MinNS: line.Range[0], MaxNS: line.Range[1]}
	if err := rng.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidLogLine, err)
	}

	sketch := NewSketchInRange(rng)
	if err := sketch.restore(line); err != nil {
		return nil, err
	}
	return sketch, nil
}

func (s *Sketch) restore(line logLine) error {
	lowest, highest := Index(s.rng.MinNS), Index(s.rng.MaxNS)
	total := line.Zeros
	for n, bucket := range line.Buckets {
		switch {
		case bucket.Index < lowest || bucket.Index > highest:
			return fmt.Errorf("%w: bucket %d is outside the range's indexes [%d, %d]",
				ErrInvalidLogLine, bucket.Index, lowest, highest)
		case n > 0 && bucket.Index <= line.Buckets[n-1].Index:
			return fmt.Errorf("%w: bucket %d is duplicated or out of order", ErrInvalidLogLine, bucket.Index)
		case bucket.Count == 0:
			return fmt.Errorf("%w: bucket %d is empty", ErrInvalidLogLine, bucket.Index)
		case total+bucket.Count < total:
			return fmt.Errorf("%w: bucket counts overflow", ErrInvalidLogLine)
		}
		total += bucket.Count
		s.buckets[bucket.Index] = bucket.Count
	}

	switch {
	case total != line.Count:
		return fmt.Errorf("%w: count %d does not match zeros plus buckets %d", ErrInvalidLogLine, line.Count, total)
	case line.Underflow > line.Count || line.Overflow > line.Count:
		return fmt.Errorf("%w: clamp counters exceed count", ErrInvalidLogLine)
	case line.Count == 0 && (line.Min != nil || line.Max != nil):
		return fmt.Errorf("%w: an empty sketch carries no min or max", ErrInvalidLogLine)
	case line.Count > 0 && (line.Min == nil || line.Max == nil):
		return fmt.Errorf("%w: min and max are required when count is positive", ErrInvalidLogLine)
	case line.Count > 0 && *line.Min > *line.Max:
		return fmt.Errorf("%w: min %d is above max %d", ErrInvalidLogLine, *line.Min, *line.Max)
	}

	s.count = line.Count
	s.sum = line.Sum
	s.zeros = line.Zeros
	s.underflowCount = line.Underflow
	s.overflowCount = line.Overflow
	if line.Count > 0 {
		s.min = *line.Min
		s.max = *line.Max
	}
	return nil
}

// MergeLogLine parses a published line and folds it into s. The line must
// resolve the same Range as s; on any error s is left untouched.
func (s *Sketch) MergeLogLine(data []byte) error {
	other, err := ParseLogLine(data)
	if err != nil {
		return err
	}
	if other.rng != s.rng {
		return fmt.Errorf("%w: line resolves [%d, %d], sketch resolves [%d, %d]",
			ErrRangeMismatch, other.rng.MinNS, other.rng.MaxNS, s.rng.MinNS, s.rng.MaxNS)
	}
	s.Merge(other)
	return nil
}
