package monitor

import (
	"sync"
	"time"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

// SchedulingStatus stores task schedule times without exposing mutable state.
type SchedulingStatus struct {
	mu       sync.RWMutex
	statuses map[schemas.TaskType]TaskSchedulingStatus
}

// SetNext records the next expected execution time.
func (s *SchedulingStatus) SetNext(task schemas.TaskType, next time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = make(map[schemas.TaskType]TaskSchedulingStatus)
	}
	status := s.statuses[task]
	status.Next = next
	s.statuses[task] = status
}

// SetLast records the most recent execution time.
func (s *SchedulingStatus) SetLast(task schemas.TaskType, last time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statuses == nil {
		s.statuses = make(map[schemas.TaskType]TaskSchedulingStatus)
	}
	status := s.statuses[task]
	status.Last = last
	s.statuses[task] = status
}

// Snapshot returns a detached copy of all scheduling times.
func (s *SchedulingStatus) Snapshot() map[schemas.TaskType]TaskSchedulingStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy := make(map[schemas.TaskType]TaskSchedulingStatus, len(s.statuses))
	for task, status := range s.statuses {
		copy[task] = status
	}
	return copy
}
