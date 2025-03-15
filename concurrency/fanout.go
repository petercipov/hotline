package concurrency

import (
	"context"
	"fmt"
	"hash/fnv"
)

type contextProcessId struct{}

var contextProcessIdName = contextProcessId{}

func GetProcessIdFromContext(ctx context.Context) string {
	name, found := ctx.Value(contextProcessIdName).(string)
	if found {
		return name
	} else {
		return ""
	}
}

type FanOut[M any] struct {
	channels []chan M
}

func NewFanOut[M any](process func(ctx context.Context, m M), numberOfQueues int) *FanOut[M] {
	channels := make([]chan M, numberOfQueues)
	for i := range numberOfQueues {
		inputChannel := make(chan M)
		channels[i] = inputChannel
		processId := fmt.Sprintf("fan%d", i)
		go func(messages chan M, processID string) {
			ctx := context.WithValue(context.Background(), contextProcessIdName, processID)
			for message := range messages {
				process(ctx, message)
			}
		}(inputChannel, processId)
	}

	return &FanOut[M]{
		channels: channels,
	}
}

func (f *FanOut[M]) Send(id []byte, m M) {
	if len(f.channels) == 0 {
		return
	}

	index := 0
	hash := fnv.New32()
	_, hashErr := hash.Write(id)
	if hashErr == nil {
		idHash := int(hash.Sum32())
		index = idHash % len(f.channels)
	}
	f.channels[index] <- m
}

func (f *FanOut[M]) Broadcast(m M) {
	for i := range f.channels {
		f.channels[i] <- m
	}
}

func (f *FanOut[M]) Close() {
	for i := range f.channels {
		close(f.channels[i])
	}
}
