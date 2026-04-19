package search

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fixtureCase struct {
	Name                string            `json:"name"`
	Prompt              string            `json:"prompt"`
	ExpectedSurviving   []fixtureSurvivor `json:"expected_surviving"`
	ExpectedSuppression string            `json:"expected_suppression"`
}

type fixtureSurvivor struct {
	Name    string   `json:"name"`
	Matched []string `json:"matched"`
}

type fixtureFile struct {
	Fixtures []fixtureCase `json:"fixtures"`
}

// TestPipelineFixtures runs every fixture against the pipeline and asserts
// expected surviving entries + suppression reason. The fixture file is the
// source of truth for "correct" pipeline behavior.
//
// Regenerate with:
//
//	UPDATE_FIXTURES=1 go test ./internal/search/ -run TestPipelineFixtures
//
// Commit the regenerated testdata/pipeline_fixtures.json. If a fixture does
// not exercise the gate it was designed for, fix the corpus in
// fixture_corpus_test.go, regenerate, and verify the gate fires.
func TestPipelineFixtures(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "pipeline_fixtures.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var ff fixtureFile
	if err := json.Unmarshal(raw, &ff); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}

	corpus := pipelineFixtureCorpus()
	stopWords := DefaultStopWords()

	if os.Getenv("UPDATE_FIXTURES") == "1" {
		for i, fx := range ff.Fixtures {
			ff.Fixtures[i] = regenerateFixture(fx, corpus, stopWords)
		}
		writeFixtures(t, path, ff)
		return
	}

	for _, fx := range ff.Fixtures {
		fx := fx
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			tokens := Tokenize(fx.Prompt, stopWords)
			pr := Run(PipelineInput{
				Tokens:     tokens,
				Corpus:     corpus,
				Threshold:  DefaultThreshold,
				Limit:      DefaultLimit,
				ApplyGates: true,
			})
			assertFixture(t, fx, pr)
		})
	}
}

// assertFixture checks surviving entries and suppression reason against fixture expectations.
// Cardinality mismatch is a fast-fail; individual mismatches report all actual results so
// the operator can diagnose without re-running.
func assertFixture(t *testing.T, fx fixtureCase, pr PipelineResult) {
	t.Helper()

	gotSupp := string(pr.Suppression)
	if gotSupp != fx.ExpectedSuppression {
		t.Errorf("suppression: got %q, want %q\n  surviving: %s",
			gotSupp, fx.ExpectedSuppression, formatSurviving(pr.Surviving))
	}

	if len(pr.Surviving) != len(fx.ExpectedSurviving) {
		t.Errorf("surviving count: got %d, want %d\n  got:  %s\n  want: %s",
			len(pr.Surviving), len(fx.ExpectedSurviving),
			formatSurviving(pr.Surviving),
			formatExpected(fx.ExpectedSurviving))
		return // cardinality mismatch: skip per-entry checks
	}

	for i := range pr.Surviving {
		got := pr.Surviving[i]
		want := fx.ExpectedSurviving[i]

		if got.Entry.Name != want.Name {
			t.Errorf("[%d] name: got %q, want %q\n  surviving: %s",
				i, got.Entry.Name, want.Name, formatSurviving(pr.Surviving))
		}
		if !slices.Equal(got.Matched, want.Matched) {
			t.Errorf("[%d] %s matched: got %v, want %v",
				i, got.Entry.Name, got.Matched, want.Matched)
		}
	}
}

// regenerateFixture runs the pipeline for a fixture case and updates its
// expected values from live output. Used by UPDATE_FIXTURES=1 mode.
func regenerateFixture(fx fixtureCase, corpus *InMemoryCorpus, stops StopSet) fixtureCase {
	tokens := Tokenize(fx.Prompt, stops)
	pr := Run(PipelineInput{
		Tokens:     tokens,
		Corpus:     corpus,
		Threshold:  DefaultThreshold,
		Limit:      DefaultLimit,
		ApplyGates: true,
	})

	fx.ExpectedSuppression = string(pr.Suppression)
	fx.ExpectedSurviving = make([]fixtureSurvivor, len(pr.Surviving))
	for i, s := range pr.Surviving {
		matched := s.Matched
		if matched == nil {
			matched = []string{}
		}
		fx.ExpectedSurviving[i] = fixtureSurvivor{
			Name:    s.Entry.Name,
			Matched: matched,
		}
	}
	return fx
}

// writeFixtures marshals and atomically writes the fixture file, sorted by name.
func writeFixtures(t *testing.T, path string, ff fixtureFile) {
	t.Helper()

	slices.SortFunc(ff.Fixtures, func(a, b fixtureCase) int {
		return strings.Compare(a.Name, b.Name)
	})

	data, err := json.MarshalIndent(ff, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixtures: %v", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write tmp fixtures: %v", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename fixtures: %v", err)
	}
	t.Logf("fixtures regenerated: %s", path)
}

// formatSurviving renders a ScoredEntry slice for error messages.
func formatSurviving(entries []ScoredEntry) string {
	if len(entries) == 0 {
		return "[]"
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = fmt.Sprintf("{name:%q matched:%v score:%d}", e.Entry.Name, e.Matched, e.Score)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatExpected renders a fixtureSurvivor slice for error messages.
func formatExpected(survivors []fixtureSurvivor) string {
	if len(survivors) == 0 {
		return "[]"
	}
	parts := make([]string, len(survivors))
	for i, s := range survivors {
		parts[i] = fmt.Sprintf("{name:%q matched:%v}", s.Name, s.Matched)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
