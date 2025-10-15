package servicelevels

const httpRange1xx = "1xx"
const httpRange2xx = "2xx"
const httpRange3xx = "3xx"
const httpRange4xx = "4xx"
const httpRange5xx = "5xx"

const httpRangeUnknown = "range_unknown"

type HttpStateRangeBreakdown struct {
	states map[string]string
	ranges []string
}

func NewHttpStateRangeBreakdown() *HttpStateRangeBreakdown {
	ranges := []string{
		httpRange1xx,
		httpRange2xx,
		httpRange3xx,
		httpRange4xx,
		httpRange5xx,
		httpRangeUnknown,
	}
	return &HttpStateRangeBreakdown{
		states: map[string]string{
			"100": httpRange1xx,
			"101": httpRange1xx,
			"102": httpRange1xx,
			"103": httpRange1xx,

			"200": httpRange2xx,
			"201": httpRange2xx,
			"202": httpRange2xx,
			"203": httpRange2xx,
			"204": httpRange2xx,
			"205": httpRange2xx,
			"206": httpRange2xx,
			"207": httpRange2xx,
			"208": httpRange2xx,
			"226": httpRange2xx,

			"300": httpRange3xx,
			"301": httpRange3xx,
			"302": httpRange3xx,
			"303": httpRange3xx,
			"304": httpRange3xx,
			"305": httpRange3xx,
			"306": httpRange3xx,
			"307": httpRange3xx,
			"308": httpRange3xx,

			"400": httpRange4xx,
			"401": httpRange4xx,
			"402": httpRange4xx,
			"403": httpRange4xx,
			"404": httpRange4xx,
			"405": httpRange4xx,
			"406": httpRange4xx,
			"407": httpRange4xx,
			"408": httpRange4xx,
			"409": httpRange4xx,
			"410": httpRange4xx,
			"411": httpRange4xx,
			"412": httpRange4xx,
			"413": httpRange4xx,
			"414": httpRange4xx,
			"415": httpRange4xx,
			"416": httpRange4xx,
			"417": httpRange4xx,
			"418": httpRange4xx,
			"421": httpRange4xx,
			"422": httpRange4xx,
			"423": httpRange4xx,
			"424": httpRange4xx,
			"425": httpRange4xx,
			"426": httpRange4xx,
			"428": httpRange4xx,
			"429": httpRange4xx,
			"431": httpRange4xx,
			"451": httpRange4xx,

			"500": httpRange5xx,
			"501": httpRange5xx,
			"502": httpRange5xx,
			"503": httpRange5xx,
			"504": httpRange5xx,
			"505": httpRange5xx,
			"506": httpRange5xx,
			"507": httpRange5xx,
			"508": httpRange5xx,
			"510": httpRange5xx,
			"511": httpRange5xx,
		},
		ranges: ranges,
	}
}

func (b *HttpStateRangeBreakdown) GetRanges() []string {
	return b.ranges
}

func (b *HttpStateRangeBreakdown) ConvertStateToRange(state string) *string {
	httpRange, found := b.states[state]
	if !found {
		return nil
	}
	return &httpRange
}
