package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	releaseAPIURL   = "https://api.github.com/repos/dotcommander/claudette/releases/latest"
	goInstallTarget = "github.com/dotcommander/claudette/cmd/claudette@latest"
)

// resolveReleaseAPIURL returns the GitHub releases API URL, overridable via
// CLAUDETTE_RELEASE_API_URL for tests (points at httptest.Server).
func resolveReleaseAPIURL() string {
	if u := os.Getenv("CLAUDETTE_RELEASE_API_URL"); u != "" {
		return u
	}
	return releaseAPIURL
}

// fetchLatestTag queries the GitHub releases API and returns the latest tag_name.
// Caps response body at 1 MiB (releases payload is ~5 KiB; limit is a safety net).
func fetchLatestTag(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolveReleaseAPIURL(), nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "claudette")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %s from releases API", resp.Status)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode release payload: %w", err)
	}
	return payload.TagName, nil
}

// compareVersions returns a human-readable status string describing how current
// relates to latest. Non-vX.Y.Z current strings (dev builds, commit hashes) are
// "ahead of released"; mismatched vX.Y.Z strings are "behind".
func compareVersions(current, latest string) string {
	if current == latest {
		return "up to date"
	}
	if isReleasedVersion(current) {
		return "run 'claudette update' to upgrade"
	}
	return "ahead of released"
}

// isReleasedVersion returns true when s looks like vX.Y.Z (no pre-release suffix,
// no commit hash). Implemented without regexp to keep deps at cobra + yaml.v3.
func isReleasedVersion(s string) bool {
	if !strings.HasPrefix(s, "v") {
		return false
	}
	rest := s[1:] // strip leading 'v'
	// Must be exactly two dots, digits only between them, no hyphens.
	if strings.Count(rest, ".") != 2 {
		return false
	}
	if strings.ContainsAny(rest, "-+") {
		return false
	}
	for _, c := range rest {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// CheckUpdate queries the GitHub releases API for the latest tag and prints a
// single-line comparison against currentVersion to w. Never spawns a subprocess.
func CheckUpdate(ctx context.Context, w io.Writer, currentVersion string) error {
	latest, err := fetchLatestTag(ctx)
	if err != nil {
		return fmt.Errorf("checking latest release: %w", err)
	}
	status := compareVersions(currentVersion, latest)
	fmt.Fprintf(w, "current: %s, latest: %s — %s\n", currentVersion, latest, status)
	return nil
}

// Update runs `go install github.com/dotcommander/claudette/cmd/claudette@latest`,
// streaming the subprocess's stdout/stderr to stderr so the user sees real-time
// download/build progress. Returns the subprocess's exit error unchanged.
func Update(ctx context.Context, stderr io.Writer) error {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return errors.New("go toolchain not found in PATH — install from https://go.dev/dl/ then re-run 'claudette update'")
	}

	fmt.Fprintf(stderr, "Running: go install %s\n", goInstallTarget)

	cmd := exec.CommandContext(ctx, goPath, "install", goInstallTarget)
	cmd.Stdout = stderr
	cmd.Stderr = stderr
	return cmd.Run()
}
