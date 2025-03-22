package concurrency

import "context"

type ScopeWorkers[S any, W any, M any] struct {
	workers      map[string]*scopedWorker[S, W]
	inputChannel chan *M
}

type scopedWorker[S any, W any] struct {
	worker *W
	scope  *Scope[S]
}

func NewScopeWorkers[W any, S any, M any](
	scopes *Scopes[S],
	workerCreator func(cxt context.Context, scope *S) *W,
	executor func(ctx context.Context, scope *S, worker *W, message *M),
	inputChannelLength int,
) *ScopeWorkers[S, W, M] {
	inputChannel := make(chan *M, inputChannelLength)
	workers := make(map[string]*scopedWorker[S, W], scopes.Len())
	for scopeID, scope := range scopes.ForEachScope() {
		worker := workerCreator(scope.Ctx, scope.Value)
		workers[scopeID] = &scopedWorker[S, W]{
			worker: worker,
			scope:  scope,
		}
		go func(scope *Scope[S], worker *W) {
			for msg := range inputChannel {
				executor(scope.Ctx, scope.Value, worker, msg)
			}
		}(scope, worker)
	}
	return &ScopeWorkers[S, W, M]{
		workers:      workers,
		inputChannel: inputChannel,
	}
}

func (w *ScopeWorkers[S, W, M]) Execute(m *M) {
	w.inputChannel <- m
}

func (w *ScopeWorkers[S, W, M]) Close() {
	close(w.inputChannel)
}
