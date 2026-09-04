package ai

import (
	"errors"
	"fmt"
	"slices"
	"sync"
)

type SessionResourceCleanup func(...string)

var sessionResources struct {
	sync.Mutex
	cleanups []*SessionResourceCleanup
}

func RegisterSessionResourceCleanup(cleanup SessionResourceCleanup) (func(), error) {
	if cleanup == nil {
		return nil, errors.New("session resource cleanup must not be nil")
	}
	sessionResources.Lock()
	sessionResources.cleanups = append(sessionResources.cleanups, &cleanup)
	sessionResources.Unlock()
	return func() {
		sessionResources.Lock()
		sessionResources.cleanups = slices.DeleteFunc(sessionResources.cleanups, func(entry *SessionResourceCleanup) bool { return entry == &cleanup })
		sessionResources.Unlock()
	}, nil
}

// CleanupSessionResources snapshots registrations; callbacks run without the
// registry lock and must synchronize their own resources across concurrent calls.
func CleanupSessionResources(sessionID ...string) error {
	sessionResources.Lock()
	cleanups := slices.Clone(sessionResources.cleanups)
	sessionResources.Unlock()
	var failures []error
	for _, cleanup := range cleanups {
		if err := runSessionResourceCleanup(*cleanup, sessionID); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("failed to cleanup session resources: %w", errors.Join(failures...))
	}
	return nil
}

func runSessionResourceCleanup(cleanup SessionResourceCleanup, sessionID []string) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			if cause, ok := failure.(error); ok {
				err = cause
			} else {
				err = fmt.Errorf("session resource cleanup panicked: %v", failure)
			}
		}
	}()
	cleanup(slices.Clone(sessionID)...)
	return nil
}
