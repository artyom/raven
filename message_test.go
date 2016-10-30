package raven

import (
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
)

func TestNewEvent(t *testing.T) {
	const funcName = "failFoo"
	cl := &Client{tags: map[string]string{
		"Foo": "fooVal",
		"Bar": "barVal",
	}}
	msg := newMessage("test error message", "", []interface{}{1, true, failFoo()}, cl)
	t.Logf("Marshalled event representation:\n%s\n", msg.payload)
	var unp ravenEventExamine
	if err := json.Unmarshal(msg.payload, &unp); err != nil {
		t.Fatal(err)
	}
	if got, want := len(unp.Tags), len(cl.tags); got != want {
		t.Fatalf("wrong number of tags in event: want %d, got %d", want, got)
	}
	for k, v := range cl.tags {
		if v2, ok := unp.Tags[k]; !ok || v != v2 {
			t.Errorf("wrong value for event tag %q: got %q, want %q", k, v2, v)
		}
	}
	if got := unp.Culprit; got != funcName {
		t.Fatalf("wrong culprit field in event: want %q, got %q", funcName, got)
	}
	if l := len(unp.Exceptions); l != 1 {
		t.Fatalf("wrong number of exceptions in event: want 1, got %d", l)
	}
	exc := unp.Exceptions[0]
	if exc.Trace == nil {
		t.Fatalf("no trace attached to first exception")
	}
	if l := len(exc.Trace.Frames); l != maxFrames {
		t.Fatalf("wrong number of frames in first exception: want %d, got %d", maxFrames, l)
	}
	if fr := exc.Trace.Frames[0]; fr.Function != funcName {
		t.Fatalf("wrong function name in first frame of first exception: want %q, got %q",
			funcName, fr.Function)
	}
}

func failFoo() error { return errors.New("boom") }

// ravenEventExamine used to unpack marshalled wire-format event to verify its
// fields
type ravenEventExamine struct {
	Culprit    string            `json:"culprit"`
	Tags       map[string]string `json:"tags,omitempty"`
	Exceptions []struct {
		Type  string `json:"type"`
		Text  string `json:"value"`
		Trace *struct {
			Frames []struct {
				File     string `json:"filename"`
				Function string `json:"function"`
				Line     int    `json:"lineno"`
			} `json:"frames"`
		} `json:"stacktrace,omitempty"`
	} `json:"exception,omitempty"`
}
