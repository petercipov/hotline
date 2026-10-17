package ddsketch_test

import (
	"bytes"
	"encoding/json"
	"hotline/metrics/ddsketch"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Log line", func() {
	sut := logLineSut{}

	Context("encoding", func() {
		It("writes one line of JSON with no newline", func() {
			sut.forValues(normativeDataset())
			line := sut.encode()
			Expect(bytes.ContainsAny(line, "\r\n")).To(BeFalse())
			Expect(json.Valid(line)).To(BeTrue())
		})

		It("pins the format of a small sketch", func() {
			sketch := ddsketch.NewSketch()
			sketch.Insert(500_000)
			sketch.Insert(10_000_000)
			sketch.Insert(10_000_000)
			sketch.Insert(16 * 60 * 1_000_000_000)

			line, err := sketch.MarshalLogLine()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(line)).To(Equal(
				`{"v":1,"range":[1000000,900000000000],"count":4,"sum":960020500000,` +
					`"min":500000,"max":960000000000,"zeros":1,"underflow":1,"overflow":1,` +
					`"buckets":[[403,2],[689,1]]}`))
		})

		It("writes the same line for the same state whatever the insertion order", func() {
			sut.forValues(normativeDataset())
			forward := sut.encode()

			values := normativeDataset()
			for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
				values[i], values[j] = values[j], values[i]
			}
			sut.forValues(values)
			Expect(sut.encode()).To(Equal(forward))
		})

		It("leaves min and max out of an empty sketch", func() {
			line, err := ddsketch.NewSketch().MarshalLogLine()
			Expect(err).NotTo(HaveOccurred())
			Expect(string(line)).To(Equal(
				`{"v":1,"range":[1000000,900000000000],"count":0,"sum":0,` +
					`"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`))
		})
	})

	Context("round trip", func() {
		It("restores an empty sketch exactly", func() {
			sut.forEmptySketch()
			Expect(sut.roundTrip().Equal(sut.sketch)).To(BeTrue())
		})

		It("restores the normative dataset exactly", func() {
			sut.forValues(normativeDataset())
			Expect(sut.roundTrip().Equal(sut.sketch)).To(BeTrue())
		})

		It("restores random sketches exactly", func() {
			for seed := uint64(1); seed <= 20; seed++ {
				sut.sketch = randomSketch(seed, 500)
				Expect(sut.roundTrip().Equal(sut.sketch)).To(BeTrue())
			}
		})

		It("restores a sketch over a custom range", func() {
			sut.sketch = ddsketch.NewSketchInRange(ddsketch.Range{MinNS: 1, MaxNS: 1_000})
			sut.sketch.Insert(0)
			sut.sketch.Insert(7)
			sut.sketch.Insert(5_000)
			restored := sut.roundTrip()
			Expect(restored.Range()).To(Equal(ddsketch.Range{MinNS: 1, MaxNS: 1_000}))
			Expect(restored.Equal(sut.sketch)).To(BeTrue())
		})
	})

	Context("merging ingested lines", func() {
		It("gives the same sketch as merging the originals", func() {
			left := randomSketch(1, 300)
			right := randomSketch(2, 300)
			expected := left.Clone()
			expected.Merge(right)

			line, err := right.MarshalLogLine()
			Expect(err).NotTo(HaveOccurred())
			merged := left.Clone()
			Expect(merged.MergeLogLine(line)).To(Succeed())
			Expect(merged.Equal(expected)).To(BeTrue())
		})

		It("folds many lines in any order to the same sketch", func() {
			lines := [][]byte{}
			for seed := uint64(1); seed <= 5; seed++ {
				line, err := randomSketch(seed, 200).MarshalLogLine()
				Expect(err).NotTo(HaveOccurred())
				lines = append(lines, line)
			}

			forward := ddsketch.NewSketch()
			for _, line := range lines {
				Expect(forward.MergeLogLine(line)).To(Succeed())
			}
			backward := ddsketch.NewSketch()
			for i := len(lines) - 1; i >= 0; i-- {
				Expect(backward.MergeLogLine(lines[i])).To(Succeed())
			}
			Expect(forward.Equal(backward)).To(BeTrue())
			Expect(forward.Count()).To(Equal(uint64(1000)))
		})

		It("rejects a line over a different range and leaves the sketch untouched", func() {
			other := ddsketch.NewSketchInRange(ddsketch.Range{MinNS: 1, MaxNS: 1_000})
			other.Insert(10)
			line, err := other.MarshalLogLine()
			Expect(err).NotTo(HaveOccurred())

			sut.forValues(normativeDataset())
			before := sut.sketch.Clone()
			Expect(sut.sketch.MergeLogLine(line)).To(MatchError(ddsketch.ErrRangeMismatch))
			Expect(sut.sketch.Equal(before)).To(BeTrue())
		})

		It("rejects a malformed line and leaves the sketch untouched", func() {
			sut.forValues(normativeDataset())
			before := sut.sketch.Clone()
			Expect(sut.sketch.MergeLogLine([]byte(`{"v":1,`))).To(MatchError(ddsketch.ErrInvalidLogLine))
			Expect(sut.sketch.Equal(before)).To(BeTrue())
		})
	})

	Context("parsing", func() {
		valid := `{"v":1,"range":[1000000,900000000000],"count":3,"sum":30500000,` +
			`"min":500000,"max":20000000,"zeros":1,"underflow":1,"overflow":0,` +
			`"buckets":[[403,1],[420,1]]}`

		It("accepts a well formed line", func() {
			sketch, err := ddsketch.ParseLogLine([]byte(valid))
			Expect(err).NotTo(HaveOccurred())
			Expect(sketch.Count()).To(Equal(uint64(3)))
			Expect(sketch.Buckets()).To(Equal(map[int32]uint64{403: 1, 420: 1}))
		})

		It("accepts a trailing newline, as read from a log file", func() {
			_, err := ddsketch.ParseLogLine([]byte(valid + "\n"))
			Expect(err).NotTo(HaveOccurred())
		})

		invalid := map[string]string{
			"not JSON":            `ddsketch`,
			"two values":          valid + valid,
			"unknown version":     `{"v":2,"range":[1000000,900000000000],"count":0,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
			"missing version":     `{"range":[1000000,900000000000],"count":0,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
			"unknown field":       `{"v":1,"range":[1000000,900000000000],"count":0,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[],"extra":1}`,
			"invalid range":       `{"v":1,"range":[0,900000000000],"count":0,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
			"missing range":       `{"v":1,"count":0,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
			"count mismatch":      `{"v":1,"range":[1000000,900000000000],"count":5,"sum":1,"min":1,"max":1,"zeros":1,"underflow":1,"overflow":0,"buckets":[]}`,
			"zero bucket count":   `{"v":1,"range":[1000000,900000000000],"count":1,"sum":1,"min":1,"max":1,"zeros":1,"underflow":1,"overflow":0,"buckets":[[403,0]]}`,
			"duplicate bucket":    `{"v":1,"range":[1000000,900000000000],"count":2,"sum":2,"min":1,"max":1,"zeros":0,"underflow":0,"overflow":0,"buckets":[[403,1],[403,1]]}`,
			"unsorted buckets":    `{"v":1,"range":[1000000,900000000000],"count":2,"sum":2,"min":1,"max":1,"zeros":0,"underflow":0,"overflow":0,"buckets":[[420,1],[403,1]]}`,
			"short bucket":        `{"v":1,"range":[1000000,900000000000],"count":1,"sum":1,"min":1,"max":1,"zeros":0,"underflow":0,"overflow":0,"buckets":[[403]]}`,
			"min above max":       `{"v":1,"range":[1000000,900000000000],"count":1,"sum":1,"min":9,"max":1,"zeros":1,"underflow":1,"overflow":0,"buckets":[]}`,
			"missing min":         `{"v":1,"range":[1000000,900000000000],"count":1,"sum":1,"max":1,"zeros":1,"underflow":1,"overflow":0,"buckets":[]}`,
			"min on empty sketch": `{"v":1,"range":[1000000,900000000000],"count":0,"sum":0,"min":1,"max":1,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
			"negative count":      `{"v":1,"range":[1000000,900000000000],"count":-1,"sum":0,"zeros":0,"underflow":0,"overflow":0,"buckets":[]}`,
		}

		for name, line := range invalid {
			It("rejects "+name, func() {
				_, err := ddsketch.ParseLogLine([]byte(line))
				Expect(err).To(MatchError(ddsketch.ErrInvalidLogLine))
			})
		}
	})
})

type logLineSut struct {
	sketch *ddsketch.Sketch
}

func (s *logLineSut) forEmptySketch() {
	s.sketch = ddsketch.NewSketch()
}

func (s *logLineSut) forValues(values []uint64) {
	s.sketch = ddsketch.NewSketch()
	for _, v := range values {
		s.sketch.Insert(v)
	}
}

func (s *logLineSut) encode() []byte {
	line, err := s.sketch.MarshalLogLine()
	Expect(err).NotTo(HaveOccurred())
	return line
}

func (s *logLineSut) roundTrip() *ddsketch.Sketch {
	restored, err := ddsketch.ParseLogLine(s.encode())
	Expect(err).NotTo(HaveOccurred())
	return restored
}
