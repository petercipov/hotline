package concurrency

import "context"

type ScopeWorkers[S any, W any, M any] struct {
	workers      map[ScopeID]*scopedWorker[S, W]
	inputChannel chan *M
}

type scopedWorker[S any, W any] struct {
	worker *W
	scope  Scope[S]
}

func NewScopeWorkers[W any, S any, M any](
	scopes *Scopes[S],
	workerCreator func(id ScopeID, scope *S) *W,
	executor func(ctx context.Context, id string, scope *S, worker *W, message *M),
	inputChannelLength int,
) *ScopeWorkers[S, W, M] {
	inputChannel := make(chan *M, inputChannelLength)
	workers := make(map[ScopeID]*scopedWorker[S, W], scopes.Len())
	for _, scope := range scopes.ForEachScope() {
		worker := workerCreator(scope.ID, scope.Value)
		workers[scope.ID] = &scopedWorker[S, W]{
			worker: worker,
			scope:  scope,
		}
		go func(scope Scope[S], worker *W) {
			runContext := context.Background()
			id := string(scope.ID)
			for msg := range inputChannel {
				executor(runContext, id, scope.Value, worker, msg)
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
