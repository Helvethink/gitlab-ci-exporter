package logger

import (
	"fmt"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Attribute keys follow the OpenTelemetry semantic conventions used by the
// former github.com/uptrace/opentelemetry-go-extra/otellogrus hook, so that
// existing trace queries keep working.
const (
	attrLogSeverity      = attribute.Key("log.severity")
	attrLogMessage       = attribute.Key("log.message")
	attrCodeFunction     = attribute.Key("code.function")
	attrCodeFilepath     = attribute.Key("code.filepath")
	attrCodeLineNumber   = attribute.Key("code.lineno")
	attrExceptionType    = attribute.Key("exception.type")
	attrExceptionMessage = attribute.Key("exception.message")
)

// OtelHook is a logrus hook that records log entries as events on the span
// carried by the entry's context (set via log.WithContext). Entries at or
// above ErrorLevel also mark the span status as an error.
type OtelHook struct {
	levels           []log.Level
	errorStatusLevel log.Level
}

var _ log.Hook = (*OtelHook)(nil)

// NewOtelHook returns a hook firing for the given levels. When no level is
// provided it defaults to panic, fatal, error and warn.
func NewOtelHook(levels ...log.Level) *OtelHook {
	if len(levels) == 0 {
		levels = []log.Level{log.PanicLevel, log.FatalLevel, log.ErrorLevel, log.WarnLevel}
	}

	return &OtelHook{
		levels:           levels,
		errorStatusLevel: log.ErrorLevel,
	}
}

// Levels implements log.Hook.
func (h *OtelHook) Levels() []log.Level {
	return h.levels
}

// Fire implements log.Hook.
func (h *OtelHook) Fire(entry *log.Entry) error {
	if entry.Context == nil {
		return nil
	}

	span := trace.SpanFromContext(entry.Context)
	if !span.IsRecording() {
		return nil
	}

	attrs := make([]attribute.KeyValue, 0, len(entry.Data)+5)
	attrs = append(attrs,
		attrLogSeverity.String(levelString(entry.Level)),
		attrLogMessage.String(entry.Message),
	)

	if entry.Caller != nil {
		if entry.Caller.Function != "" {
			attrs = append(attrs, attrCodeFunction.String(entry.Caller.Function))
		}

		if entry.Caller.File != "" {
			attrs = append(attrs,
				attrCodeFilepath.String(entry.Caller.File),
				attrCodeLineNumber.Int(entry.Caller.Line),
			)
		}
	}

	for k, v := range entry.Data {
		if err, ok := v.(error); ok && k == log.ErrorKey {
			attrs = append(attrs,
				attrExceptionType.String(reflect.TypeOf(err).String()),
				attrExceptionMessage.String(err.Error()),
			)

			continue
		}

		attrs = append(attrs, toAttribute(k, v))
	}

	span.AddEvent("log", trace.WithAttributes(attrs...))

	if entry.Level <= h.errorStatusLevel {
		span.SetStatus(codes.Error, entry.Message)
	}

	return nil
}

func toAttribute(key string, value any) attribute.KeyValue {
	k := attribute.Key(key)

	switch v := value.(type) {
	case nil:
		return k.String("<nil>")
	case string:
		return k.String(v)
	case bool:
		return k.Bool(v)
	case int:
		return k.Int(v)
	case int8:
		return k.Int64(int64(v))
	case int16:
		return k.Int64(int64(v))
	case int32:
		return k.Int64(int64(v))
	case int64:
		return k.Int64(v)
	case uint8:
		return k.Int64(int64(v))
	case uint16:
		return k.Int64(int64(v))
	case uint32:
		return k.Int64(int64(v))
	case float32:
		return k.Float64(float64(v))
	case float64:
		return k.Float64(v)
	case []string:
		return k.StringSlice(v)
	case error:
		return k.String(v.Error())
	case fmt.Stringer:
		return k.String(v.String())
	default:
		return k.String(fmt.Sprint(v))
	}
}

func levelString(lvl log.Level) string {
	if lvl == log.WarnLevel {
		return "WARN"
	}

	return strings.ToUpper(lvl.String())
}
