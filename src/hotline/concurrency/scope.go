package concurrency

import (
	"iter"
	"maps"
)

type Scope[S any] struct {
	Value *S
	name  string
}

type Scopes[S any] struct {
	names  []string
	scopes map[string]*Scope[S]
}

func NewScopes[S any](names []string, createScope func() *S) *Scopes[S] {
	scopes := make(map[string]*Scope[S], len(names))
	for _, name := range names {
		scopes[name] = &Scope[S]{
			Value: createScope(),
			name:  name,
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
