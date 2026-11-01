package proxy

import (
	"net/http"
	"sync/atomic"
	"time"

	"hotline-proxy/internal/spec"
)

// specCheck is one side of a call's check against the spec, run alongside
// the call as soon as that side is whole, so it is never in the call's way
// (Peter, 2026-10-31).
type specCheck struct {
	started atomic.Bool
	done    chan struct{}
	start   time.Time
	end     time.Time
	found   spec.Checked
}

func newSpecCheck() *specCheck { return &specCheck{done: make(chan struct{})} }

// begin starts the check at start, once: the first caller wins.
func (s *specCheck) begin(start time.Time, now func() time.Time, check func() spec.Checked) {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	s.start = start
	go func() {
		s.found = check()
		s.end = now()
		close(s.done)
	}()
}

// wait waits for the check to end, and says whether it ran at all.
func (s *specCheck) wait() bool {
	if !s.started.Load() {
		return false
	}
	<-s.done
	return true
}

// checkRequest starts the check of the request.
func (c *call) checkRequest(start time.Time) {
	if c.api == nil {
		return
	}
	c.reqCheck.begin(start, c.h.clock.Now, func() spec.Checked {
		body := c.reqBody.copied()
		c.action = c.api.Spec.BodyAction(c.in.Header.Get("Content-Type"), body)
		found := c.api.Spec.CheckRequest(spec.Exchange{
			Method:      c.in.Method,
			EscapedPath: c.target.EscapedPath(),
			RawQuery:    c.target.RawQuery,
			Header:      c.in.Header,
			Body:        body,
			Action:      c.action,
		})
		c.reqBody.drop()
		return found
	})
}

// checkReply starts the check of the reply.
func (c *call) checkReply(start time.Time, resp *http.Response) {
	if c.api == nil || resp == nil {
		return
	}
	c.replyCheck.begin(start, c.h.clock.Now, func() spec.Checked {
		if c.api.Spec.NamesActionInBody() {
			// The route is known only once the request body is whole;
			// finish starts the request check for every call.
			<-c.reqCheck.done
		}
		found := c.api.Spec.CheckReply(spec.Exchange{
			Method:      c.in.Method,
			EscapedPath: c.target.EscapedPath(),
			RawQuery:    c.target.RawQuery,
			Header:      c.in.Header,
			Action:      c.action,
			Status:      resp.StatusCode,
			ReplyHeader: resp.Header,
			ReplyBody:   c.respBody.copied(),
		})
		c.respBody.drop()
		return found
	})
}
