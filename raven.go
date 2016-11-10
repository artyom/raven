// Package raven implements basic Sentry client compatible with standard logging
// facilities.
//
// Consider your program takes *log.Logger and only uses its Print, Println,
// Printf methods. Then you can update your code signature to take Logger
// interface instead of exact *log.Logger type and use this package to provide
// drop-in replacement to usual logging like this:
//
// 	logger, err := raven.New(WithDSN(os.Getenv("SENTRY_DSN")))
//	... // handle error
//
//	logger.Print("some informational message")
//
//	if err := myfunc() ; err != nil {
//		logger.Printf("myfunc failed: %v", err)
//	}
//
// Client automatically marks messages having non-nil error arguments as error
// events in Sentry; if error has stacktrace attached to it with
// github.com/pkg/errors package, this stacktrace is sent to Sentry as well.
package raven

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"
)

// Logger describes set of methods used by Client for logging; standard lib
// *log.Logger implements this interface as well.
type Logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// ConfFunc is a function mutating Client that is used for configuration when
// creating new Client. These functions should only be used as an argument to
// New() function when creating new Client, attempt to apply such functions to
// alreaty started client panics.
type ConfFunc func(*Client) (*Client, error)

// WithLogger adds Logger to Client. Client would be logging messages passed to
// it as well as delivery/overflow errors to this Logger.
func WithLogger(l Logger) ConfFunc {
	return func(c *Client) (*Client, error) {
		if c == nil {
			c = new(Client)
		}
		c.init()
		c.log = l
		return c, nil
	}
}

// WithDSN configures Client to use Sentry API endpoint specified by given DSN.
func WithDSN(dsn string) ConfFunc {
	return func(c *Client) (*Client, error) {
		apiURL, headers, err := parseDSN(dsn)
		if err != nil {
			return nil, err
		}
		if c == nil {
			c = new(Client)
		}
		c.init()
		c.apiURL = apiURL
		c.auth = headers
		return c, nil
	}
}

// WithTags configures Client to assign given set of tags to every message it
// sends.
func WithTags(tags map[string]string) ConfFunc {
	return func(c *Client) (*Client, error) {
		if c == nil {
			c = new(Client)
		}
		c.init()
		c.tags = tags
		return c, nil
	}
}

func (c *Client) init() {
	c.doInit.Do(func() {
		c.messages = make(chan *message, 1000)
		c.done = make(chan struct{})
		c.wait = make(chan struct{})
	})
	if c.started {
		panic(errRunningClientModify)
	}
}

// New returns new Client initialized with provided configuration functions.
// Basic configuration can be done using only WithDSN function:
//
// 	client, err := New(WithDSN("https://public:secret@sentry.example.com/1"))
//	...
func New(conf ...ConfFunc) (*Client, error) {
	if len(conf) == 0 {
		return nil, errors.New("no configuration functions provided")
	}
	var c *Client
	var err error
	for _, cfg := range conf {
		if c, err = cfg(c); err != nil {
			return nil, err
		}
	}
	if c.apiURL == "" || len(c.auth) == 0 {
		return nil, errors.New("DSN not configured: use WithDSN function")
	}
	if name, err := os.Hostname(); err == nil {
		c.hostname = name
	}
	hc := &http.Client{
		Timeout: 30 * time.Second,
	}
	go c.loopSend(hc)
	c.started = true
	return c, nil
}

// Client is a basic Sentry client implementing Logger interface. Consider using
// this interface in your code, as this would allow usage of stdlib log.Logger
// and Client interchangeably. Client also implements io.Writer interface
// specifically to be used as the underlying writer for log package Logger when
// it's impossible to use interface.
type Client struct {
	messages chan *message
	doInit   sync.Once     // guards Client initialize
	once     sync.Once     // guards close of done channel
	done     chan struct{} // signals termination of queue processing
	wait     chan struct{} // used to block using Wait() method
	started  bool          // if true, Client is NOT safe to be modified by ConfFunc
	isClone  bool          // true if client is a derived logger without background loop

	apiURL string   // Sentry API endpoint URL created from DSN
	auth   []string // authentication header values (public and private keys)

	tags     map[string]string // client-wide tags assigned to every message
	hostname string
	httpReq  *reqInfo
	extra    json.RawMessage

	log Logger
}

// loopSend iterates over message queue until Client is closed and sends
// messages to remote Sentry API
func (c *Client) loopSend(client *http.Client) {
	defer close(c.wait)
	var delay time.Duration
	const delayMax = 30 * time.Second
	const delayStep = 100 * time.Millisecond
	for {
		select {
		case m := <-c.messages:
			switch err := c.send(client, m); {
			case err == nil:
				if delay > 0 {
					delay -= delayStep / 3
				}
			case err == errThrottled && delay < delayMax:
				delay += delayStep
				fallthrough
			default:
				if c.log != nil {
					c.log.Printf("raven failed to send message %q: %v", m.text, err)
				}
			}
			if delay > 0 {
				time.Sleep(delay)
			}
		case <-c.done:
			return
		}
	}
}

// Print creates new event and pushes it to outgoing queue. Arguments are
// handled in the manner of fmt.Print.
func (c *Client) Print(v ...interface{}) {
	c.pushMessage(fmt.Sprint(v...), "", v)
	if c.log != nil {
		c.log.Print(v...)
	}
}

// Println creates new event and pushes it to outgoing queue. Arguments are
// handled in the manner of fmt.Println.
func (c *Client) Println(v ...interface{}) {
	c.pushMessage(fmt.Sprintln(v...), "", v)
	if c.log != nil {
		c.log.Println(v...)
	}
}

// Printf creates new event and pushes it to outgoing queue. Arguments are
// handled in the manner of fmt.Printf.
func (c *Client) Printf(format string, v ...interface{}) {
	c.pushMessage(fmt.Sprintf(format, v...), format, v)
	if c.log != nil {
		c.log.Printf(format, v...)
	}
}

// Close stops background goroutine processing message queue. Any messages
// pushed to closed Client would be discarded.
func (c *Client) Close() error {
	if c == nil || c.isClone {
		return nil
	}
	c.once.Do(func() { close(c.done) })
	return nil
}

// Wait blocks until background goroutine processing message queue returns,
// which normally happens after Close() call. This method can be used to make
// sure ongoing message delivery completes during program shutdown.
func (c *Client) Wait() { <-c.wait }

// Write implements io.Writer interface so that Client can be used as an
// underlying writer for log.Logger. It relies on log.Logger semantics that each
// logging operation makes a single call to the Writer's Write method. Write
// calls are non-blocking, they only put payload to send queue, so for cases
// where log output is followed by program termination (i.e. log.Fatal() call)
// queued but unsent output will be lost.
func (c *Client) Write(p []byte) (int, error) {
	if c == nil || len(p) == 0 {
		return len(p), nil
	}
	c.pushMessage(string(p), "", nil)
	return len(p), nil
}

// pushMessage accepts string with message body, and optional arguments list
// used to create this message string, creates new message and puts it into
// message queue in a non-blocking way. Argument list is inspected for non-nil
// error values, if any found, message severity changed to LevelError. It is
// called by Client's Logger methods.
func (c *Client) pushMessage(s, fmt string, vals []interface{}) {
	if c == nil || s == "" {
		return
	}
	select {
	case c.messages <- newMessage(s, fmt, vals, c):
	default:
		if c.log != nil {
			c.log.Print("raven queue overflow on: ", s)
		}
	}
}

// clone returns shallow copy of client
func (c *Client) clone() *Client {
	c2 := *c
	c2.isClone = true
	return &c2
}

// errRunningClientModify used as panic message thrown by ConfFuncs when they're
// applied to already initialized/started Client
const errRunningClientModify = "attempt to modify already initialized Client"
