package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		current string
		latest  string
		want    string
	}{
		{"v0.6.0", "v0.6.0", "up to date"},
		{"v0.5.0", "v0.6.0", "run 'claudette update' to upgrade"},
		// v0.7.0 is numerically newer but compareVersions uses string equality only;
		// any mismatched vX.Y.Z is treated as "behind" since we cannot distinguish
		// without semver. The "ahead of released" branch is reserved for non-vX.Y.Z.
		{"v0.7.0", "v0.6.0", "run 'claudette update' to upgrade"},
		{"dev", "v0.6.0", "ahead of released"},
		{"abc1234", "v0.6.0", "ahead of released"},
		{"abc1234-dirty", "v0.6.0", "ahead of released"},
		{"(devel)", "v0.6.0", "ahead of released"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.current+"_vs_"+tc.latest, func(t *testing.T) {
			t.Parallel()
			got := compareVersions(tc.current, tc.latest)
			if got != tc.want {
				t.Errorf("compareVersions(%q, %q) = %q, want %q", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

func TestCheckUpdate(t *testing.T) {
	// No t.Parallel() here — subtests use t.Setenv which prohibits parallel parent.

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v9.9.9"})
		}))
		defer srv.Close()

		t.Setenv("CLAUDETTE_RELEASE_API_URL", srv.URL)

		var buf bytes.Buffer
		err := CheckUpdate(context.Background(), &buf, "v0.6.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "latest: v9.9.9") {
			t.Errorf("expected 'latest: v9.9.9' in output, got: %q", out)
		}
		if !strings.Contains(out, "run 'claudette update' to upgrade") {
			t.Errorf("expected upgrade prompt in output, got: %q", out)
		}
	})

	t.Run("server_error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}))
		defer srv.Close()

		t.Setenv("CLAUDETTE_RELEASE_API_URL", srv.URL)

		err := CheckUpdate(context.Background(), &bytes.Buffer{}, "v0.6.0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "checking latest release:") {
			t.Errorf("expected 'checking latest release:' prefix, got: %v", err)
		}
	})

	t.Run("malformed_json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("not json {{{{"))
		}))
		defer srv.Close()

		t.Setenv("CLAUDETTE_RELEASE_API_URL", srv.URL)

		err := CheckUpdate(context.Background(), &bytes.Buffer{}, "v0.6.0")
		if err == nil {
			t.Fatal("expected error for malformed JSON, got nil")
		}
	})

	t.Run("context_cancelled", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"tag_name": "v9.9.9"})
		}))
		defer srv.Close()

		t.Setenv("CLAUDETTE_RELEASE_API_URL", srv.URL)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel before the call

		err := CheckUpdate(ctx, &bytes.Buffer{}, "v0.6.0")
		if err == nil {
			t.Fatal("expected error for cancelled context, got nil")
		}
	})
}

func TestUpdate_SkipsUnderShort(t *testing.T) {
	if testing.Short() {
		t.Skip("update spawns real go install — skipped in -short mode")
	}
	// Only run locally; CI runs with -short.
}
