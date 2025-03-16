package concurrency_test

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"hotline/concurrency"
	"time"
)

var _ = Describe("Fan Out", func() {
	sut := fanOutSut{}

	AfterEach(func() {
		sut.Close()
	})

	It("will not schedule message, if empty", func() {
		sut.forEmptyFanOut()
		sut.scheduleMessage()
		messages := sut.expectMessageReceived(0)
		Expect(messages).To(HaveLen(0))
	})

	It("will pass message", func() {
		sut.forSingleFanOut()
		sut.scheduleMessage()
		messages := sut.expectMessageReceived(1)
		Expect(messages).To(HaveLen(1))
	})

	It("will pass multiple message to multiple queues", func() {
		sut.forFanOut(8)
		for i := 0; i < 100000; i++ {
			sut.sendMessageWithId([]byte(fmt.Sprintf("message id %d", i)))
		}
		messages := sut.expectMessageReceived(100000)
		Expect(messages).To(HaveLen(100000))

		processIds := map[string]bool{}
		for _, message := range messages {
			processIds[message.processId] = true
		}
		Expect(processIds).To(HaveLen(8))
	})

	It("will broadcast same sage to multiple queues", func() {
		sut.forFanOut(8)
		for i := 0; i < 100; i++ {
			sut.broadcastMessageWithId([]byte(fmt.Sprintf("message id %d", i)))
		}
		messages := sut.expectMessageReceived(8 * 100)
		Expect(messages).To(HaveLen(8 * 100))

		processIds := map[string]int{}
		for _, message := range messages {
			processIds[message.processId]++
		}
		Expect(processIds).To(HaveLen(8))

		for _, count := range processIds {
			Expect(count).To(Equal(100))
		}
	})
})

type fanOutSut struct {
	fanOut *concurrency.FanOut[sutMessage, *singleWriterScope]
}

type singleWriterScope struct {
	messages []sutMessage
}

func (f *fanOutSut) forSingleFanOut() {
	f.forFanOut(1)
}

func (f *fanOutSut) forFanOut(numberOfQueues int) {
	var queueNames []string
	for i := 0; i < numberOfQueues; i++ {
		queueNames = append(queueNames, fmt.Sprintf("fan%d", i))
	}

	f.fanOut = concurrency.NewFanOut(
		queueNames,
		func(ctx context.Context, m sutMessage, scope *singleWriterScope) {
			name := concurrency.GetQueueIDFromContext(ctx)
			m.processId = name
			scope.messages = append(scope.messages, m)
		},
		func(ctx context.Context) *singleWriterScope {
			return &singleWriterScope{}
		})
}

func (f *fanOutSut) forEmptyFanOut() {
	f.forFanOut(0)
}

func (f *fanOutSut) Close() {
	f.fanOut.Close()
}

func (f *fanOutSut) scheduleMessage() {
	f.sendMessageWithId([]byte{})
}

func (f *fanOutSut) sendMessageWithId(id []byte) {
	f.fanOut.Send(id, sutMessage{
		id: id,
	})
}

func (f *fanOutSut) broadcastMessageWithId(id []byte) {
	f.fanOut.Broadcast(sutMessage{
		id: id,
	})
}

func (f *fanOutSut) expectMessageReceived(count int) []sutMessage {
	for {
		var allMessages []sutMessage
		for _, scope := range f.fanOut.Scopes() {
			allMessages = append(allMessages, scope.messages...)

		}
		if len(allMessages) >= count {
			return allMessages
		}
		time.Sleep(time.Millisecond * 1)
	}
}

type sutMessage struct {
	id        []byte
	processId string
}
