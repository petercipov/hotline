package servicelevels_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/servicelevels"
)

var _ = Describe("Tags Histogram", func() {

	s := suttagshistogram{}

	It("returns nothing when empty", func() {
		s.forEmptyHistogram()
		percentiles := s.getPercentiles()
		Expect(percentiles).To(BeNil())
	})

	It("returns nothing when empty and unknown tag is added", func() {
		s.forEmptyHistogram()
		s.Add("success")
		percentiles := s.getPercentiles()
		Expect(percentiles).To(BeNil())
	})

	It("returns percentile for defined tags if adding defined tags", func() {
		s.forHistogram("success")
		s.Add("success")
		percentiles := s.getPercentiles()
		Expect(percentiles).ToNot(BeNil())
		Expect(percentiles).To(HaveLen(1))
		Expect(percentiles[0]).To(BeNumerically("==", 100))
	})

	It("returns percentile for defined tags if adding multiple tags", func() {
		s.forHistogram("success", "failure")
		s.Add("success")
		s.Add("success")
		s.Add("success")
		s.Add("failure")
		percentiles := s.getPercentiles()
		Expect(percentiles).ToNot(BeNil())
		Expect(percentiles).To(HaveLen(2))
		Expect(percentiles[0]).To(BeNumerically("==", 75))
		Expect(percentiles[1]).To(BeNumerically("==", 25))
	})
})

type suttagshistogram struct {
	histogram *servicelevels.TagHistogram
}

func (s *suttagshistogram) forEmptyHistogram() {
	s.forHistogram()
}

func (s *suttagshistogram) getPercentiles() []float64 {
	return s.histogram.GetPercentiles()
}

func (s *suttagshistogram) forHistogram(tags ...string) {
	s.histogram = servicelevels.NewTagsHistogram(tags)
}

func (s *suttagshistogram) Add(tag string) {
	s.histogram.Add(tag)
}
