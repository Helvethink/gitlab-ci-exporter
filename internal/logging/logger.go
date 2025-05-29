package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

func NewLogger(level, format string) (logger *slog.Logger, err error) {
	var slogHandlerOptions slog.HandlerOptions
	var slogHandler slog.Handler

	switch strings.ToLower(level) {
	case "debug":
		slogHandlerOptions = slog.HandlerOptions{
			Level: slog.LevelDebug,
		}
	case "info":
		slogHandlerOptions = slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
	case "warn":
		slogHandlerOptions = slog.HandlerOptions{
			Level: slog.LevelWarn,
		}
	case "error":
		slogHandlerOptions = slog.HandlerOptions{
			Level: slog.LevelError,
		}
	default:
		slogHandlerOptions = slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
		err = fmt.Errorf("log level not recognized, falling back to info")
	}

	switch strings.ToLower(format) {
	case "text":
		slogHandler = slog.NewTextHandler(os.Stdout, &slogHandlerOptions)
	case "json":
		slogHandler = slog.NewJSONHandler(os.Stdout, &slogHandlerOptions)
	default:
		slogHandler = slog.NewTextHandler(os.Stdout, &slogHandlerOptions)
		err = fmt.Errorf("log format not recognized, falling back to text")
	}

	return slog.New(slogHandler), err
}

/*
*
var out *zap.Logger

	func init() {
		var err error
		loggerConfig := zap.NewProductionConfig()
		loggerConfig.EncoderConfig.TimeKey = "timestamp"
		loggerConfig.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)

		out, err = loggerConfig.Build(zap.AddCallerSkip(1))
		if err != nil {
			panic(err)
		}
	}


type LogLevel zapcore.Level

var (
	LevelDebug  = LogLevel(zapcore.DebugLevel)
	LevelInfo   = LogLevel(zapcore.InfoLevel)
	LevelWarn   = LogLevel(zapcore.WarnLevel)
	LevelError  = LogLevel(zapcore.ErrorLevel)
	LevelDPanic = LogLevel(zapcore.DPanicLevel)
	LevelPanic  = LogLevel(zapcore.PanicLevel)
	LevelFatal  = LogLevel(zapcore.FatalLevel)
)

// LevelFromString parses a string-based level and returns the corresponding
// LogLevel.
//
// Supported strings are: DEBUG, INFO, WARN, ERROR, DPANIC, PANIC, FATAL, and
// their lower-case forms.
//
// The returned LogLevel must be discarded if error is not nil.
func levelFromString(level string) (LogLevel, error) {
	lvl := zapcore.InfoLevel // by default value
	err := lvl.Set(level)
	return LogLevel(lvl), err
}

func CreateLogger(level string, format string) (*zap.Logger, error) {
	encodeConfig := zap.NewProductionEncoderConfig()
	encodeConfig.TimeKey = "timestamp"
	encodeConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)

	lvl, err := levelFromString(level)
	if err != nil {
		fmt.Errorf("Allowed values: DEBUG, INFO, WARN, ERROR, DPANIC, PANIC, FATAL. Values received: %s\n", level)
		return nil, err
	}

	config := zap.Config{
		Level:             zap.NewAtomicLevelAt(zapcore.Level(lvl)),
		Development:       false,
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          format,
		EncoderConfig:     encodeConfig,
		OutputPaths:       []string{"stdout"},
		ErrorOutputPaths:  []string{"stderr"},
		InitialFields: map[string]interface{}{
			"pid": os.Getpid(),
		},
	}
	return zap.Must(config.Build()), nil
}
*/
