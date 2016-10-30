package raven

import "testing"

func TestParseDSN(t *testing.T) {
	testCases := []struct {
		input string
		url   string
		hdr   []string
		bad   bool
	}{
		{"https://public:secret@sentry.example.com/1",
			"https://sentry.example.com/api/1/store/",
			[]string{"sentry_key=public", "sentry_secret=secret"},
			false},
	}
	for _, tc := range testCases {
		url, hdr, err := parseDSN(tc.input)
		if err != nil && tc.bad {
			continue
		}
		if err != nil {
			t.Fatalf("input %q failed with error: %v", tc.input, err)
		}
		if url != tc.url {
			t.Fatalf("input %q parsed to wrong url: got %q, want %q", tc.input, url, tc.url)
		}
		if len(hdr) != len(tc.hdr) {
			t.Fatalf("input %q extracted wrong headers: got %v, want %v", tc.input, hdr, tc.hdr)
		}
		for i := 0; i < len(hdr); i++ {
			if hdr[i] == tc.hdr[i] {
				continue
			}
			t.Fatalf("input %q extracted wrong headers: got %v, want %v", tc.input, hdr, tc.hdr)
		}
	}
}
