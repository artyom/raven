package raven

import "testing"

func TestNew_invalid1(t *testing.T) {
	c, err := New()
	if err == nil {
		t.Fatal("Client initialize with empty configuration succeeded")
	}
	c.Close()
}

func TestNew_invalid2(t *testing.T) {
	const dsn = "http://foo:bar@example.com/1"
	c, err := New(WithDSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	defer func() {
		reason := recover()
		if reason != errRunningClientModify {
			t.Fatal("should have panicked with: ", errRunningClientModify)
		}
	}()
	WithDSN(dsn)(c)
}
