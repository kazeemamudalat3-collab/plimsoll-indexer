package api

import (
	"io"
	"net/http"
	"testing"

	"github.com/plimsoll-protocol/plimsoll-indexer/internal/changelog"
)

// testChangelog is what the test server serves at /v1/changelog.
var testChangelog = []changelog.Entry{{
	Version: "1.2.0",
	Date:    "2026-11-20",
	Changes: []changelog.Change{{
		Kind:   "removed_field",
		Detail: "coverage.ratio was removed; compute it from bps.",
	}},
}}

// TestConditionalGet covers issue #118: single-resource endpoints carry an
// ETag, answer 304 with no body when If-None-Match names it, and still answer
// 200 with the body when it does not.
func TestConditionalGet(t *testing.T) {
	srv := server()
	defer srv.Close()

	paths := []string{
		"/v1/assets/" + sac,
		"/v1/network",
		"/v1/breakdowns/" + breakdownHash,
		"/v1/changelog",
	}
	etags := map[string]string{}
	for _, path := range paths {
		first, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(first.Body)
		first.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if first.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d want 200", path, first.StatusCode)
		}
		etag := first.Header.Get("ETag")
		if etag == "" || etag[0] != '"' || etag[len(etag)-1] != '"' {
			t.Fatalf("%s: ETag = %q, want a quoted validator", path, etag)
		}
		etags[path] = etag

		conditional := func(validator string) (status int, responseETag string, got []byte) {
			t.Helper()
			req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("If-None-Match", validator)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			got, err = io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			return resp.StatusCode, resp.Header.Get("ETag"), got
		}

		status, responseETag, got := conditional(etag)
		if status != http.StatusNotModified {
			t.Fatalf("%s: If-None-Match %s gave status %d, want 304", path, etag, status)
		}
		if len(got) != 0 {
			t.Fatalf("%s: 304 carried a %d-byte body", path, len(got))
		}
		if responseETag != etag {
			t.Fatalf("%s: 304 ETag = %q, want %q", path, responseETag, etag)
		}

		// A stale or different validator must not suppress the body.
		status, _, got = conditional(`"stale"`)
		if status != http.StatusOK {
			t.Fatalf("%s: stale validator gave status %d, want 200", path, status)
		}
		if string(got) != string(body) {
			t.Fatalf("%s: body changed between unconditional and stale-validator responses", path)
		}
	}

	// Two different representations must not share a validator.
	if etags["/v1/assets/"+sac] == etags["/v1/network"] {
		t.Fatal("asset and network responses share an ETag")
	}
}

// If-None-Match is a list of (possibly weak) validators, and "*" matches any
// current representation.
func TestIfNoneMatchForms(t *testing.T) {
	srv := server()
	defer srv.Close()

	first, err := http.Get(srv.URL + "/v1/assets/" + sac)
	if err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	etag := first.Header.Get("ETag")
	weak := "W/" + etag
	for name, header := range map[string]string{
		"list":   `"other", ` + etag,
		"weak":   weak,
		"any":    "*",
		"padded": "  " + etag + " ",
	} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/assets/"+sac, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("If-None-Match", header)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusNotModified {
				t.Fatalf("If-None-Match %q gave status %d, want 304", header, resp.StatusCode)
			}
		})
	}
}

func TestChangelogEndpoint(t *testing.T) {
	srv := server()
	defer srv.Close()

	body := get(t, srv, "/v1/changelog", 200)
	entries, ok := body["changelog"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("changelog = %#v", body["changelog"])
	}
	entry := entries[0].(map[string]any)
	if entry["version"] != "1.2.0" || entry["date"] != "2026-11-20" {
		t.Fatalf("entry = %#v", entry)
	}
	changes, ok := entry["changes"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("changes = %#v", entry["changes"])
	}
	change := changes[0].(map[string]any)
	if change["kind"] != "removed_field" || change["detail"] == "" {
		t.Fatalf("change = %#v", change)
	}
}

// Without a changelog loaded the endpoint still answers with a JSON array, so
// clients never have to special-case null.
func TestChangelogEmpty(t *testing.T) {
	srv := newTestServer(nil, nil, "", nil)
	defer srv.Close()
	body := get(t, srv, "/v1/changelog", 200)
	if entries, ok := body["changelog"].([]any); !ok || len(entries) != 0 {
		t.Fatalf("changelog = %#v", body["changelog"])
	}
}
