package raven

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// message is a queued item to be sent to Sentry API
type message struct {
	text    string // used only if send failed to log along with error
	ts      time.Time
	gzipped bool   // whether payload is gzipped
	payload []byte // json-encoded data acceptable by Sentry API
}

// newMessage returns new message created from given arguments. text is a fully
// formatted text, the one you'd usually see going to log. format is optional
// format string used to create text. vals is an optional slice of arguments
// used to create text. Created message does not retain references to vals, so
// it's safe to modify them after message is created. By default message has
// "info" severity assigned, if vals contain non-nil error value, then message
// severity set to "error" and error information is added to message.
func newMessage(text, format string, vals []interface{}, c *Client) *message {
	msg := &message{
		text: text,
		ts:   time.Now().UTC(),
	}
	evt := &event{
		ID:        randomID(),
		Text:      text,
		Timestamp: msg.ts.Format(sentryTimeFormat),
		Level:     levelInfo,
		Platform:  "go",
	}
	if c != nil {
		evt.Tags = c.tags
		evt.Hostname = c.hostname
		evt.Request = c.httpReq
		evt.Extra = c.extra
	}
	if format != "" && len(vals) > 0 {
		evt.Details = &details{Format: format, Text: text}
	}
	var errs []error
	for _, v := range vals {
		if evt.Details != nil {
			evt.Details.Params = append(evt.Details.Params,
				fmt.Sprint(v))
		}
		switch err := v.(type) {
		case error:
			if err != nil {
				errs = append(errs, err)
				evt.Level = levelError
			}
		}
	}
	if len(errs) > 0 {
		if e, ok := errors.Cause(errs[0]).(stackTracer); ok {
			if st := e.StackTrace(); len(st) > 0 {
				evt.Culprit = fmt.Sprintf("%n", st[0])
			}
		}
	}
	for _, err := range errs {
		evt.Exceptions = append(evt.Exceptions, ravenException{err})
	}
	if data, err := json.Marshal(evt); err == nil {
		msg.payload = data
	}
	return msg
}

func (c *Client) send(hc *http.Client, msg *message) error {
	if len(msg.payload) == 0 {
		return errors.New("empty message payload")
	}
	var err error
	var doSleep bool
	for wait := 200 * time.Millisecond; wait < 3*time.Second; wait *= 2 {
		switch {
		case doSleep:
			time.Sleep(wait)
		default:
			doSleep = true
		}
		req, err := http.NewRequest(http.MethodPost, c.apiURL, bytes.NewReader(msg.payload))
		if err != nil {
			return err
		}
		req.Header.Add("User-Agent", userAgent)
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add(authHeader, "Sentry sentry_version=7")
		req.Header.Add(authHeader, fmt.Sprintf("sentry_timestamp=%d", msg.ts.Unix()))
		for _, h := range c.auth {
			req.Header.Add(authHeader, h)
		}
		if err = doRequest(hc, req); err == nil {
			return nil
		}
		if e, ok := err.(temporary); ok && e.Temporary() {
			continue
		}
		return err
	}
	return err
}

func doRequest(hc *http.Client, req *http.Request) error {
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch x := resp.StatusCode; {
	case x == http.StatusOK:
		return nil
	case x == http.StatusTooManyRequests:
		return errThrottled
	case http.StatusBadRequest <= x && x < http.StatusInternalServerError:
		errText := "Sentry API request error: "
		if reason := resp.Header.Get(sentryErrorHeader); reason != "" {
			return temporaryError(errText + reason)
		} else {
			return temporaryError(errText + resp.Status)
		}
	case x >= http.StatusInternalServerError:
		return permanentError("Sentry API server error: " + resp.Status)
	}
	return nil
}

type temporary interface {
	Temporary() bool
}

type permanentError string

func (e permanentError) Error() string   { return string(e) }
func (e permanentError) Temporary() bool { return true }

type temporaryError string

func (e temporaryError) Error() string   { return string(e) }
func (e temporaryError) Temporary() bool { return false }

var errThrottled = temporaryError("throttle required, Sentry API overloaded")

const (
	sentryErrorHeader = "X-Sentry-Error"
	authHeader        = "X-Sentry-Auth"
	userAgent         = "github.com/artyom/raven"
)

const maxFrames = 3 // max. number of frames to include per single error

func randomID() string {
	b := make([]byte, 16) // TODO: pool this
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", b)
}

type stackTracer interface {
	StackTrace() errors.StackTrace
}
