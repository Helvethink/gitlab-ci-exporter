package monitor

import "time" // Package for time-related operations

// TaskSchedulingStatus represents the scheduling status of a task.
// It includes information about the last and next scheduled times.
type TaskSchedulingStatus struct {
	Last time.Time // The last time the task was scheduled or executed
	Next time.Time // The next time the task is scheduled to be executed
}
