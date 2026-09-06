package codingagent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"syscall"
)

var fileMutationQueues = struct {
	sync.Mutex
	tails map[string]chan struct{}
}{tails: make(map[string]chan struct{})}

// WithFileMutationQueue holds a file's FIFO slot until fn settles, even after cancellation.
func WithFileMutationQueue[T any](ctx context.Context, filePath string, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, errors.New("file mutation context must not be nil")
	}
	fileMutationQueues.Lock()
	key, err := filepath.Abs(filePath)
	if err == nil {
		var real string
		real, err = filepath.EvalSymlinks(key)
		if err == nil {
			key = real
		} else if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ENOTDIR) {
			err = nil
		}
	}
	if err != nil {
		fileMutationQueues.Unlock()
		return zero, err
	}
	previous := fileMutationQueues.tails[key]
	next := make(chan struct{})
	fileMutationQueues.tails[key] = next
	fileMutationQueues.Unlock()
	if previous != nil {
		<-previous
	}
	defer func() {
		fileMutationQueues.Lock()
		close(next)
		if fileMutationQueues.tails[key] == next {
			delete(fileMutationQueues.tails, key)
		}
		fileMutationQueues.Unlock()
	}()
	if err := context.Cause(ctx); err != nil {
		return zero, err
	}
	return fn(ctx)
}
