package concurrency

import (
	"fmt"
)

type ScopeID string
type Scope[S any] struct {
	Value *S
	ID    ScopeID
}

type Scopes[S any] struct {
	scopes []Scope[S]
}

func GenerateScopeIds(prefix string, n int) []ScopeID {
	var scopeIDs []ScopeID
	for i := range n {
		scopeIDs = append(scopeIDs, ScopeID(fmt.Sprintf("%s-%d", prefix, i)))
	}
	return scopeIDs
}

func NewScopes[S any](names []ScopeID, createScope func() *S) *Scopes[S] {
	scopes := make([]Scope[S], len(names))
	for i, name := range names {
		scopes[i] = Scope[S]{
			Value: createScope(),
			ID:    name,
		}
	}
	return &Scopes[S]{
		scopes: scopes,
	}
}

func (s *Scopes[S]) Len() int {
	return len(s.scopes)
}

func (s *Scopes[S]) ForEachScope() []Scope[S] {
	return s.scopes
}
