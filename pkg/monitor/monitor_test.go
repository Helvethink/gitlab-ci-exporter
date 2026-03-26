package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskSchedulingStatus(t *testing.T) {
	last := time.Unix(1710000000, 0)
	next := last.Add(5 * time.Minute)

	status := TaskSchedulingStatus{
		Last: last,
		Next: next,
	}

	assert.Equal(t, last, status.Last)
	assert.Equal(t, next, status.Next)
}
