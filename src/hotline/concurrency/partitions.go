package concurrency

import "context"

type ShardingKey []byte

type Message interface {
	GetShardingKey() ShardingKey
}

type PartitionPublisher interface {
	PublishToPartition(ctx context.Context, message Message)
}

type PartitionConsumer[S any] interface {
	ConsumeFromPartition(ctx context.Context, partitionID string, message Message, scope *S)
}

type ScopedAction[S any] interface {
	Execute(ctx context.Context, scopeID string, scope *S)
}
