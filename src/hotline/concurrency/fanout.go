package concurrency

import (
	"context"
	"hash/fnv"
)

type FanOut[M any, S any] struct {
	channels []chan M
	scopes   *Scopes[S]
}

func NewFanOut[M any, S any](scopes *Scopes[S], queueProcessor func(ctx context.Context, scopeID string, m M, scope *S)) *FanOut[M, S] {
	channels := make([]chan M, scopes.Len())
	for i := range channels {
		channels[i] = make(chan M)
	}

	i := 0
	for queueID, scope := range scopes.ForEachScope() {
		go func(messages chan M, id string, queueScope *S) {
			runContext := context.Background()
			for message := range messages {
				queueProcessor(runContext, id, message, queueScope)
			}
		}(channels[i], queueID, scope.Value)
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

type InMessageConsumer[S any] struct {
	PartitionConsumer[S]
}

func (m *InMessageConsumer[S]) ConsumeFromPartition(ctx context.Context, partitionID string, message Message, scope *S) {
	action := message.(ScopedAction[S])
	action.Execute(ctx, partitionID, scope)
}

func NewFanoutWithMessagesConsumer[S any](scopes *Scopes[S]) *FanOut[Message, S] {
	c := &InMessageConsumer[S]{}
	f := NewFanOut(scopes, c.ConsumeFromPartition)
	return f
}

type FanoutPublisher[S any] struct {
	PartitionPublisher
	fanout *FanOut[Message, S]
}

func NewFanoutPublisher[S any](fanout *FanOut[Message, S]) *FanoutPublisher[S] {
	return &FanoutPublisher[S]{fanout: fanout}
}

func (p *FanoutPublisher[S]) PublishToPartition(_ context.Context, message Message) {
	key := message.GetShardingKey()
	if key != nil {
		p.fanout.Send(key, message)
	} else {
		p.fanout.Broadcast(message)
	}
}
