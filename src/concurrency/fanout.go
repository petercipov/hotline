package concurrency

import (
	"context"
	"hash/fnv"
)

type FanOut[M any, S any] struct {
	channels []chan M
	scopes   *Scopes[S]
}

func NewFanOut[M any, S any](scopes *Scopes[S], queueProcessor func(ctx context.Context, m M, scope *S)) *FanOut[M, S] {
	channels := make([]chan M, scopes.Len())
	i := 0
	for queueID, scope := range scopes.ForEachScope() {
		messages := make(chan M)
		channels[i] = messages
		go func(ctx context.Context, messages chan M, processID string, queueScope *S) {
			for message := range messages {
				queueProcessor(ctx, message, queueScope)
			}
		}(scope.Ctx, messages, queueID, scope.Value)
		i++
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

func (f *FanOut[M, S]) Close() {
	for i := range f.channels {
		close(f.channels[i])
	}
	f.scopes = nil
}
