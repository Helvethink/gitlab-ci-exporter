package monitor

import (
	"sync"
	"testing"
	"time"

	"github.com/helvethink/gitlab-ci-exporter/pkg/schemas"
)

func TestSchedulingStatusSnapshotIsDetachedUnderConcurrentUpdates(t *testing.T) {
	var status SchedulingStatus
	task := schemas.TaskTypePullMetrics
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				status.SetLast(task, time.Now())
				status.SetNext(task, time.Now().Add(time.Second))
			}
		}()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				snapshot := status.Snapshot()
				delete(snapshot, task)
			}
		}()
	}
	wg.Wait()
	if _, ok := status.Snapshot()[task]; !ok {
		t.Fatal("mutating a snapshot removed the monitored task")
	}
}
