package ui

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if any test leaves a goroutine running.
//
// ratecounter's decay goroutine stops by itself once the counted rate falls back
// to zero (within one interval), so it is transient rather than leaked.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/paulbellamy/ratecounter.(*RateCounter).run.func1"),
	)
}
