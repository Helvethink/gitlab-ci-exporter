package controller

import (
	"sync"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

type taskKey struct {
	typeName schemas.TaskType
	id       string
}

// taskAdmission bounds outstanding jobs, including jobs being published.
type taskAdmission struct {
	mu       sync.Mutex
	slots    chan struct{}
	accepted map[taskKey]struct{}
}

func newTaskAdmission(capacity int) *taskAdmission {
	if capacity < 1 {
		capacity = 1
	}
	return &taskAdmission{
		slots:    make(chan struct{}, capacity),
		accepted: make(map[taskKey]struct{}),
	}
}

func (a *taskAdmission) acquire() bool {
	select {
	case a.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *taskAdmission) discard() {
	<-a.slots
}

func (a *taskAdmission) track(key taskKey) {
	a.mu.Lock()
	a.accepted[key] = struct{}{}
	a.mu.Unlock()
}

func (a *taskAdmission) release(key taskKey) {
	a.mu.Lock()
	_, ok := a.accepted[key]
	if ok {
		delete(a.accepted, key)
	}
	a.mu.Unlock()
	if ok {
		a.discard()
	}
}
