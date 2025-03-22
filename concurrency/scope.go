package concurrency

import (
	"context"
	"iter"
	"maps"
)

type contextQueueID struct{}

var contextQueueIDName = contextQueueID{}

func GetScopeIDFromContext(ctx context.Context) string {
	name, _ := ctx.Value(contextQueueIDName).(string)
	return name
}

type Scope[S any] struct {
	Ctx   context.Context
	Value *S
}

type Scopes[S any] struct {
	names  []string
	scopes map[string]*Scope[S]
}

func NewScopes[S any](names []string, createScope func(ctx context.Context) *S) *Scopes[S] {
	scopes := make(map[string]*Scope[S], len(names))
	for _, name := range names {
		ctx := context.WithValue(context.Background(), contextQueueIDName, name)
		scopes[name] = &Scope[S]{
			Ctx:   ctx,
			Value: createScope(ctx),
		}
	}
	return &Scopes[S]{
		names:  names,
		scopes: scopes,
	}
}

func (s *Scopes[S]) Len() int {
	return len(s.names)
}

func (s *Scopes[S]) ForEachScope() iter.Seq2[string, *Scope[S]] {
	return maps.All(s.scopes)
}
