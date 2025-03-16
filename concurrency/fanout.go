package concurrency

import (
	"context"
	"hash/fnv"
	"iter"
	"maps"
)

type contextQueueID struct{}

var contextQueueIDName = contextQueueID{}

func GetQueueIDFromContext(ctx context.Context) string {
	name, _ := ctx.Value(contextQueueIDName).(string)
	return name
}

type FanOut[M any, S any] struct {
	channels []chan M
	scopes   map[string]S
}

func NewFanOut[M any, S any](idsOfQueues []string, queueProcessor func(ctx context.Context, m M, scope S), createScope func(ctx context.Context) S) *FanOut[M, S] {
	channels := make([]chan M, len(idsOfQueues))
	scopes := make(map[string]S, len(idsOfQueues))
	for i, queueID := range idsOfQueues {
		ctx := context.WithValue(context.Background(), contextQueueIDName, queueID)
		scopes[queueID] = createScope(ctx)
		messages := make(chan M)
		channels[i] = messages
		go func(ctx context.Context, messages chan M, processID string, queueScope S) {
			for message := range messages {
				queueProcessor(ctx, message, queueScope)
			}
		}(ctx, messages, queueID, scopes[queueID])
	}

	return &FanOut[M, S]{
		channels: channels,
		scopes:   scopes,
	}
}

func (f *FanOut[M, S]) Send(id []byte, m M) {
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

func (f *FanOut[M, S]) Broadcast(m M) {
	for i := range f.channels {
		f.channels[i] <- m
	}
}

func (f *FanOut[M, S]) Scopes() iter.Seq2[string, S] {
	return maps.All(f.scopes)
}

func (f *FanOut[M, S]) Close() {
	for i := range f.channels {
		close(f.channels[i])
	}
	f.scopes = map[string]S{}
}
