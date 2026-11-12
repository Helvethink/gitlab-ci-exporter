package server

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package if any test leaves a goroutine running.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
