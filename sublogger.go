package raven

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
)

// AttachRequestInfo returns sublogger that sends given http.Request information
// with every message it logs. If Logger is not a *Client (i.e. it is
// *log.Logger), this function returns logger itself.
func AttachRequestInfo(l Logger, r *http.Request) Logger {
	c, ok := l.(*Client)
	if !ok {
		return l
	}
	u := new(url.URL)
	*u = *r.URL
	u.Host = r.Host
	u.Scheme = "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		u.Scheme = "https"
	}
	req := &reqInfo{
		URL:     u.String(),
		Method:  r.Method,
		Query:   r.URL.RawQuery,
		Headers: make(map[string]string, len(r.Header)),
	}
	for k, v := range r.Header {
		req.Headers[k] = strings.Join(v, ", ")
	}
	c2 := c.clone()
	c2.httpReq = req
	return c2
}

// AttachTags returns sublogger that sends additional tags for every message it
// logs. If logger is not *Client, original logger is returned.
func AttachTags(l Logger, tags map[string]string) Logger {
	c, ok := l.(*Client)
	if !ok || len(tags) == 0 {
		return l
	}
	c2 := c.clone()
	c2.tags = make(map[string]string, len(c.tags)+len(tags))
	for k, v := range c.tags {
		c2.tags[k] = v
	}
	for k, v := range tags {
		c2.tags[k] = v
	}
	return c2
}

// AttachExtra returns sublogger that sends an arbitrary mapping of additional
// metadata for every message it logs. This function calls json.Marshal on
// provided interface{} and does not retain pointers to it. If logger is not
// a *Client or data cannot be marshalled, original logger is returned.
func AttachExtra(l Logger, extra interface{}) Logger {
	c, ok := l.(*Client)
	if !ok {
		return l
	}
	data, err := json.Marshal(extra)
	if err != nil {
		return l
	}
	c2 := c.clone()
	c2.extra = data
	return c2
}
