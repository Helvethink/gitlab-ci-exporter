package cmd

import (
	"bytes"
	stdlog "log"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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

func TestLevelWriterLogsLinesAtLevel(t *testing.T) {
	var out bytes.Buffer
	logger := log.StandardLogger()
	previousOut, previousLevel, previousFormatter := logger.Out, logger.GetLevel(), logger.Formatter
	logger.SetOutput(&out)
	logger.SetLevel(log.InfoLevel)
	logger.SetFormatter(&log.TextFormatter{DisableTimestamp: true, DisableColors: true})
	t.Cleanup(func() {
		logger.SetOutput(previousOut)
		logger.SetLevel(previousLevel)
		logger.SetFormatter(previousFormatter)
	})

	stdlog.New(levelWriter{level: log.WarnLevel}, "taskq ", 0).Print("queue is full")
	stdlog.New(levelWriter{level: log.DebugLevel}, "taskq ", 0).Print("hidden")

	assert.Equal(t, "level=warning msg=\"taskq queue is full\"\n", out.String())
}
