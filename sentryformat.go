package raven

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
)

const sentryTimeFormat = "2006-01-02T15:04:05"

// event represents message format expected by Sentry
type event struct {
	ID        string   `json:"event_id"`
	Text      string   `json:"message"`
	Timestamp string   `json:"timestamp"`
	Level     severity `json:"level,omitempty"`
	Culprit   string   `json:"culprit,omitempty"`

	// https://docs.sentry.io/clientdev/interfaces/exception/
	Exceptions exceptions `json:"exception,omitempty"`

	// https://docs.sentry.io/clientdev/interfaces/message/
	Details *details `json:"logentry,omitempty"`
}

type details struct {
	Text   string   `json:"formatted"`
	Format string   `json:"message"`
	Params []string `json:"params"`
}

type exceptions []ravenException

func (e *exceptions) MarshalJSON() ([]byte, error) {
	interm := struct {
		Vals []ravenException `json:"values"`
	}{Vals: *e}
	return json.Marshal(interm)
}

type ravenException struct {
	err error
}

func (e *ravenException) MarshalJSON() ([]byte, error) {
	type frame struct {
		File string `json:"filename,omitempty"`
		Func string `json:"function,omitempty"`
		Line int    `json:"lineno"`
	}
	type stackTrace struct {
		Frames []frame `json:"frames"`
	}
	interm := struct {
		Type  string      `json:"type"`
		Text  string      `json:"value"`
		Trace *stackTrace `json:"stacktrace,omitempty"`
	}{
		Type: "error",
		Text: e.err.Error(),
	}
	if e, ok := errors.Cause(e.err).(stackTracer); ok {
		interm.Trace = new(stackTrace)
		for i, st := range e.StackTrace() {
			if i > maxFrames-1 {
				break
			}
			fr := frame{
				File: fmt.Sprintf("%s", st),
				Func: fmt.Sprintf("%n", st),
			}
			if n, err := strconv.Atoi(fmt.Sprintf("%d", st)); err == nil {
				fr.Line = n
			}
			interm.Trace.Frames = append(interm.Trace.Frames, fr)
		}
	}
	return json.Marshal(interm)
}

// severity is a Sentry log entry level
type severity int

const (
	levelFatal severity = 1 + iota
	levelError
	levelWarning
	levelInfo
	levelDebug
)

var levels = [...]string{
	"fatal",
	"error",
	"warning",
	"info",
	"debug",
}

func (s severity) String() string { return levels[s-1] }

func (s severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }
