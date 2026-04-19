package output

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dotcommander/claudette/internal/index"
	"github.com/dotcommander/claudette/internal/search"
)

// fixedReport returns a deterministic ExplainReport for golden-file and JSON shape tests.
func fixedReport() ExplainReport {
	return ExplainReport{
		Prompt:       "goroutine leak",
		RawTokens:    []string{"goroutine", "leak"},
		KeptTokens:   []string{"goroutine", "leak"},
		DroppedStops: nil,
		AvgFieldLen:  10.0,
		HasIDF:       false,
		TotalScored:  2,
		Diagnostics: []search.EntryDiagnostics{
			{
				Entry: index.Entry{
					Type:     index.TypeKB,
					Name:     "goroutine-leak-detection",
					Title:    "Goroutine Leak Detection",
					Category: "go",
					FilePath: "kb/go/goroutine-leak-detection.md",
				},
				RawScore:    5.00,
				FinalScore:  5,
				PreBoostSum: 5.00,
				UsageBoost:  1.00,
				MaxIDF:      1.0,
				TokenHits: []search.TokenHit{
					{Token: "goroutine", Kind: "alias+direct", Weight: 3, IDF: 1.0, Delta: 5.0, AliasCat: "go"},
					{Token: "leak", Kind: "direct", Weight: 2, IDF: 1.0, Delta: 2.0},
				},
				BigramHits:   []string{"goroutine leak"},
				BigramDeltas: []float64{1.5}, // max(bigramFloor=1.5, (idf1+idf2)/2) with nil IDF → 1.0 avg → floor wins
				Suppressed:   "",
			},
			{
				Entry: index.Entry{
					Type:     index.TypeKB,
					Name:     "refactoring-smells",
					Title:    "Refactoring Smells",
					Category: "refactoring",
					FilePath: "kb/refactoring/smells.md",
				},
				RawScore:    1.80,
				FinalScore:  2,
				PreBoostSum: 1.80,
				UsageBoost:  1.00,
				MaxIDF:      0.0,
				TokenHits: []search.TokenHit{
					{Token: "goroutine", Kind: "miss"},
					{Token: "leak", Kind: "miss"},
				},
				BigramHits: nil,
				Suppressed: "below threshold",
			},
		},
	}
}

// TestWriteExplainText_Golden compares text output to the golden file.
// Run with -update to regenerate: go test -run TestWriteExplainText_Golden -update
func TestWriteExplainText_Golden(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	WriteExplainText(&buf, fixedReport())
	got := buf.Bytes()

	goldenPath := filepath.Join("testdata", "explain_basic.txt")

	// Write mode: regenerate golden file.
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("writing golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v (run UPDATE_GOLDEN=1 go test to generate)", goldenPath, err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("output does not match golden file %s\n\ngot:\n%s\nwant:\n%s",
			goldenPath, got, want)
	}
}

// TestExplainReport_MarshalJSON verifies the wire shape has the documented structure.
func TestExplainReport_MarshalJSON(t *testing.T) {
	t.Parallel()

	r := fixedReport()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Top-level keys: prompt, tokens, corpus, diagnostics.
	for _, key := range []string{"prompt", "tokens", "corpus", "diagnostics"} {
		if _, ok := parsed[key]; !ok {
			t.Errorf("JSON missing top-level key %q", key)
		}
	}

	// tokens sub-object: raw, kept, dropped_stops.
	tokens, _ := parsed["tokens"].(map[string]any)
	if tokens == nil {
		t.Fatal("tokens is not an object")
	}
	for _, key := range []string{"raw", "kept", "dropped_stops"} {
		if _, ok := tokens[key]; !ok {
			t.Errorf("tokens missing key %q", key)
		}
	}

	// corpus sub-object: entries_scored, entries_shown, idf_enabled, avg_field_len.
	corpus, _ := parsed["corpus"].(map[string]any)
	if corpus == nil {
		t.Fatal("corpus is not an object")
	}
	for _, key := range []string{"entries_scored", "entries_shown", "idf_enabled", "avg_field_len"} {
		if _, ok := corpus[key]; !ok {
			t.Errorf("corpus missing key %q", key)
		}
	}

	// prompt must be the literal string.
	if got, _ := parsed["prompt"].(string); got != "goroutine leak" {
		t.Errorf("prompt=%q, want %q", got, "goroutine leak")
	}

	// diagnostics must be an array.
	diags, _ := parsed["diagnostics"].([]any)
	if diags == nil {
		t.Fatal("diagnostics is not an array")
	}
}

// TestWriteExplainText_EmptyDiagnostics verifies the formatter still shows tokens +
// corpus sections when there are no diagnostics, plus a "(no matching entries)" note.
func TestWriteExplainText_EmptyDiagnostics(t *testing.T) {
	t.Parallel()

	r := ExplainReport{
		Prompt:      "xyzzy",
		RawTokens:   []string{"xyzzy"},
		KeptTokens:  []string{"xyzzy"},
		AvgFieldLen: 5.0,
		HasIDF:      false,
		TotalScored: 0,
		Diagnostics: nil,
	}

	var buf bytes.Buffer
	WriteExplainText(&buf, r)
	out := buf.String()

	for _, want := range []string{"tokens", "corpus", "(no matching entries)"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q; got:\n%s", want, out)
		}
	}
}
