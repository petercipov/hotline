package concurrency_test

import (
	"context"
	"hotline/concurrency"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Scope Workers", func() {
	sut := workersSUT{}

	AfterEach(func() {
		sut.Close()
	})

	It("should execute anything if no workers", func() {
		sut.forNoWorkers()
		sut.execute()
		executed := sut.getExecuted()
		Expect(executed).To(BeEmpty())
	})

	It("should execute 1 for single worker", func() {
		sut.forWorker(1)
		sut.execute()
		executed := sut.ExpectExecuted(1)
		Expect(executed).To(HaveLen(1))
	})

	It("should execute multiple for in multiple workers", func() {
		sut.forWorker(10)
		for i := 0; i < 10000; i++ {
			sut.execute()
		}
		executed := sut.ExpectExecuted(10000)
		histogram := make(map[string]int)
		for _, message := range executed {
			histogram[message.workerID]++
		}
		Expect(histogram).To(HaveLen(10))
	})
})

type workersSUT struct {
	scopes  *concurrency.Scopes[workerSUTScope]
	workers *concurrency.ScopeWorkers[workerSUTScope, workerSUTWorker, workerSUTMessage]
}

type workerSUTScope struct {
	messages []*workerSUTMessage
}

type workerSUTWorker struct {
}

type workerSUTMessage struct {
	workerID string
}

func (s *workersSUT) forNoWorkers() {
	s.forWorker(0)
}

func (s *workersSUT) execute() {
	s.workers.Execute(&workerSUTMessage{})
}

func (s *workersSUT) getExecuted() []*workerSUTMessage {
	var allMessages []*workerSUTMessage
	for _, scope := range s.scopes.ForEachScope() {
		allMessages = append(allMessages, scope.Value.messages...)
	}

	return allMessages
}

func (s *workersSUT) Close() {
	s.workers.Close()
}

func (s *workersSUT) forWorker(count int) {
	var workerIDs []string

	for i := 0; i < count; i++ {
		workerIDs = append(workerIDs, strconv.Itoa(i))
	}

	s.scopes = concurrency.NewScopes(workerIDs, func() *workerSUTScope {
		return &workerSUTScope{}
	})
	s.workers = concurrency.NewScopeWorkers(
		s.scopes,
		func(_ string, scope *workerSUTScope) *workerSUTWorker {
			return &workerSUTWorker{}
		},
		func(ctx context.Context, queueID string, scope *workerSUTScope, worker *workerSUTWorker, message *workerSUTMessage) {
			message.workerID = queueID
			scope.messages = append(scope.messages, message)
			time.Sleep(1 * time.Microsecond)
		},
		10,
	)
}

func (s *workersSUT) ExpectExecuted(count int) []*workerSUTMessage {
	for {
		executed := s.getExecuted()
		if len(executed) >= count {
			return executed
		}
		time.Sleep(1 * time.Millisecond)
	}
}
