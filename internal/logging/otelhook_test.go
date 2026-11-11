package logger

import (
	"context"
	"errors"
	"io"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newHookedLogger() *log.Logger {
	l := log.New()
	l.SetOutput(io.Discard)
	l.AddHook(NewOtelHook())

	return l
}

func TestOtelHookRecordsSpanEvent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")

	newHookedLogger().WithContext(ctx).
		WithError(errors.New("boom")).
		WithField("project", "group/repo").
		WithField("count", 3).
		Error("failed")

	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, "failed", spans[0].Status().Description)

	events := spans[0].Events()
	require.Len(t, events, 1)
	assert.Equal(t, "log", events[0].Name)

	got := map[attribute.Key]attribute.Value{}
	for _, kv := range events[0].Attributes {
		got[kv.Key] = kv.Value
	}

	assert.Equal(t, "ERROR", got[attrLogSeverity].AsString())
	assert.Equal(t, "failed", got[attrLogMessage].AsString())
	assert.Equal(t, "boom", got[attrExceptionMessage].AsString())
	assert.Equal(t, "*errors.errorString", got[attrExceptionType].AsString())
	assert.Equal(t, "group/repo", got["project"].AsString())
	assert.Equal(t, int64(3), got["count"].AsInt64())
}

func TestOtelHookWarnDoesNotSetErrorStatus(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	ctx, span := tp.Tracer("test").Start(context.Background(), "op")
	newHookedLogger().WithContext(ctx).Warn("careful")
	span.End()

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Unset, spans[0].Status().Code)
	require.Len(t, spans[0].Events(), 1)
	assert.Equal(t, "WARN", spans[0].Events()[0].Attributes[0].Value.AsString())
}

func TestOtelHookIgnoresEntriesWithoutRecordingSpan(t *testing.T) {
	hook := NewOtelHook()

	assert.NoError(t, hook.Fire(&log.Entry{Message: "no context"}))
	assert.NoError(t, hook.Fire(&log.Entry{Context: context.Background(), Message: "no span"}))
}

func TestToAttribute(t *testing.T) {
	assert.Equal(t, attribute.Bool("b", true), toAttribute("b", true))
	assert.Equal(t, attribute.Float64("f", 1.5), toAttribute("f", 1.5))
	assert.Equal(t, attribute.String("e", "x"), toAttribute("e", errors.New("x")))
	assert.Equal(t, attribute.String("n", "<nil>"), toAttribute("n", nil))
	assert.Equal(t, attribute.String("s", "[1 2]"), toAttribute("s", []int{1, 2}))
}
