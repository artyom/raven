package raven

import (
	"errors"
	"net/url"
	"path"
	"strconv"
)

// parseDSN parses Sentry DSN specification returning url endpoint,
// X-Sentry-Auth authentication header values with public and secret keys and
// error, if any.
//
// For parsing logic see
// https://docs.sentry.io/clientdev/overview/#parsing-the-dsn
func parseDSN(dsn string) (string, []string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", nil, err
	}
	api := new(url.URL)
	switch u.Scheme {
	case "http", "https":
		api.Scheme = u.Scheme
	default:
		return "", nil, errors.New("unsupported DSN scheme")
	}
	if u.Host == "" {
		return "", nil, errors.New("empty DSN host")
	}
	api.Host = u.Host
	dir, project := path.Split(u.Path)
	if dir == "" {
		return "", nil, errors.New("empty DSN path")
	}
	if project == "" {
		return "", nil, errors.New("empty DSN project")
	}
	if _, err := strconv.Atoi(project); err != nil {
		return "", nil, errors.New("bad DNS project value")
	}
	if u.User == nil {
		return "", nil, errors.New("empty DSN credentials")
	}
	if u.User.Username() == "" {
		return "", nil, errors.New("empty DSN public key")
	}
	headers := make([]string, 0, 2)
	headers = append(headers, "sentry_key="+u.User.Username())
	switch p, _ := u.User.Password(); p {
	case "":
		return "", nil, errors.New("empty DSN private key")
	default:
		headers = append(headers, "sentry_secret="+p)
	}
	api.Path = path.Join(dir, "api", project, "store") + "/"
	return api.String(), headers, nil
}
