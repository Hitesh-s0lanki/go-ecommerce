// Package logger builds the application's structured logger.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New returns a logger for the given gin mode. Release mode emits JSON for log
// aggregators; other modes emit human-readable console output.
//
// The returned logger is also installed as zerolog's global logger, so
// packages using zerolog/log directly stay consistent with it.
func New(ginMode string) zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339

	return newWithWriter(ginMode, os.Stdout)
}

func newWithWriter(ginMode string, out io.Writer) zerolog.Logger {
	w := out
	if ginMode != "release" {
		w = zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	}

	l := zerolog.New(w).With().Timestamp().Logger()
	if ginMode == "release" {
		l = l.Level(zerolog.InfoLevel)
	} else {
		l = l.Level(zerolog.DebugLevel)
	}

	zerolog.DefaultContextLogger = &l

	return l
}
