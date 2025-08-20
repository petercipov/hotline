package servicelevels_test

import (
	"hotline/servicelevels"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tags Histogram", func() {

	s := suttagshistogram{}

	It("returns nothing when empty", func() {
		s.forEmptyHistogram()
		percentiles := s.getPercentile("unknown")
		Expect(percentiles).To(BeNil())
	})

	It("returns nothing when empty and unknown tag is added", func() {
		s.forEmptyHistogram()
		s.Add("success")
		percentiles := s.getPercentile("success")
		Expect(percentiles).To(BeNil())
	})

	It("returns percentile for defined tags if adding defined tags", func() {
		s.forHistogram("success")
		s.Add("success")
		percentiles := s.getPercentile("success")
		Expect(percentiles).ToNot(BeNil())
		Expect(*percentiles).To(BeNumerically("==", 100))
	})

	It("returns percentile for defined tags if adding multiple tags", func() {
		s.forHistogram("success", "failure")
		s.Add("success")
		s.Add("success")
		s.Add("success")
		s.Add("failure")

		Expect(*s.getPercentile("success")).To(BeNumerically("==", 75))
		Expect(*s.getPercentile("failure")).To(BeNumerically("==", 25))
	})
})

type suttagshistogram struct {
	histogram *servicelevels.TagHistogram
}

func (s *suttagshistogram) forEmptyHistogram() {
	s.forHistogram()
}

func (s *suttagshistogram) getPercentile(tag string) *float64 {
	p, _ := s.histogram.ComputePercentile(tag)
	return p
}

func (s *suttagshistogram) forHistogram(tags ...string) {
	s.histogram = servicelevels.NewTagsHistogram(tags)
}

func (s *suttagshistogram) Add(tag string) {
	s.histogram.Add(tag)
}
