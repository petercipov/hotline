package concurrency_test

import (
	"context"
	"fmt"
	"hotline/concurrency"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		Expect(messages).To(BeEmpty())
	})

	It("will pass message", func() {
		sut.forSingleFanOut()
		sut.scheduleMessage()
		messages := sut.expectMessageReceived(1)
		Expect(messages).To(HaveLen(1))
	})

	It("will pass multiple message to multiple queues", func() {
		sut.forFanOut(8)
		for i := range 100000 {
			sut.sendMessageWithId(fmt.Sprintf("message id %d", i))
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
		for i := range 100 {
			sut.broadcastMessageWithId(fmt.Sprintf("message id %d", i))
		}
		received := sut.expectMessageReceived(8 * 100)
		Expect(received).To(HaveLen(8 * 100))

		byProcessId := map[string][]sutMessage{}
		for _, message := range received {
			byProcessId[message.processId] = append(byProcessId[message.processId], message)
		}
		Expect(byProcessId).To(HaveLen(8))
		for _, messages := range byProcessId {
			count := len(messages)
			Expect(count).To(Equal(100))
		}
	})
})

type fanOutSut struct {
	scopes *concurrency.Scopes[singleWriterScope]
	fanOut *concurrency.FanOut[concurrency.ScopedAction[singleWriterScope], singleWriterScope]
}

type singleWriterScope struct {
	messages []sutMessage
}

func (f *fanOutSut) forSingleFanOut() {
	f.forFanOut(1)
}

func (f *fanOutSut) forFanOut(numberOfQueues int) {
	f.scopes = concurrency.NewScopes(concurrency.GenerateScopeIds("fan", numberOfQueues), func() *singleWriterScope {
		return &singleWriterScope{}
	})
	f.fanOut = concurrency.NewActionFanOut(f.scopes)
}

func (f *fanOutSut) forEmptyFanOut() {
	f.forFanOut(0)
}

func (f *fanOutSut) Close() {
	f.fanOut.Close()
	f.fanOut = nil
}

func (f *fanOutSut) scheduleMessage() {
	f.sendMessageWithId("")
}

func (f *fanOutSut) sendMessageWithId(id string) {
	f.fanOut.Send([]byte(id), &sutMessage{
		id: id,
	})
}

func (f *fanOutSut) broadcastMessageWithId(id string) {
	f.fanOut.Broadcast(&sutMessage{
		id: id,
	})
}

func (f *fanOutSut) expectMessageReceived(count int) []sutMessage {
	for {
		var allMessages []sutMessage
		for _, scope := range f.scopes.ForEachScope() {
			allMessages = append(allMessages, scope.Value.messages...)
		}
		if len(allMessages) >= count {
			return allMessages
		}
		time.Sleep(time.Millisecond * 1)
	}
}

type sutMessage struct {
	id        string
	processId string
}

func (m *sutMessage) Execute(_ context.Context, scopeID string, scope *singleWriterScope) {
	m.processId = scopeID
	scope.messages = append(scope.messages, *m)
}
