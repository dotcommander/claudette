package search

import "github.com/dotcommander/claudette/internal/index"

// pipelineFixtureCorpus returns a 13-entry synthetic corpus shaped to exercise
// every scoring path: IDF variation, alias boost, bigram bonus, and the three
// gates (low-confidence, single-token-floor, differential).
//
// Shared keywords that drive IDF variation:
//
//	goroutine — entries 1,2,12 (df=3, IDF≈1.36)
//	channel   — entries 2,12 (df=2, IDF≈1.59)
//	refactor  — entries 3,9 (df=2, IDF≈1.59)
//
// All other keywords are unique (df=1, IDF=2.0).
func pipelineFixtureCorpus() *InMemoryCorpus {
	return NewInMemoryCorpus(
		// ── Strong signal: multi-token + bigram match ──────────────────────────
		// Fixture: clear_multi_token_match / alias_boost
		index.Entry{
			Type:     index.TypeKB,
			Name:     "goroutine-leak-detection",
			Title:    "Goroutine Leak Detection",
			Category: "go",
			Desc:     "Detect goroutine leaks via goleak in integration tests",
			Keywords: map[string]int{"goroutine": 3, "leak": 3, "goleak": 2, "detection": 2},
			Bigrams:  []string{"goroutine leak"},
		},

		// ── Differential pair ─────────────────────────────────────────────────
		// These two entries are designed to score close together on "goroutine channel"
		// (both match goroutine+channel via direct keywords + bigram).
		// Fixture: differential_gap
		index.Entry{
			Type:     index.TypeKB,
			Name:     "go-concurrency",
			Title:    "Go Concurrency Patterns",
			Category: "go",
			Desc:     "Goroutine and channel concurrency patterns",
			Keywords: map[string]int{"concurrency": 3, "goroutine": 2, "channel": 2},
			Bigrams:  []string{"goroutine channel"},
		},
		index.Entry{
			Type:     index.TypeKB,
			Name:     "go-channels",
			Title:    "Go Channel Usage",
			Category: "go",
			Desc:     "Channel-based communication between goroutines",
			Keywords: map[string]int{"goroutine": 2, "channel": 2, "pipeline": 1},
			Bigrams:  []string{"goroutine channel"},
		},

		// ── Alias boost fixture ───────────────────────────────────────────────
		// Single-token-floor: "golang" triggers alias+direct on this entry.
		// Score lands in [4,7] with one matched token → single-token-floor fires.
		index.Entry{
			Type:     index.TypeKB,
			Name:     "golang-patterns",
			Title:    "Golang Coding Patterns",
			Category: "go",
			Desc:     "Idiomatic golang patterns and conventions",
			Keywords: map[string]int{"golang": 2, "convention": 1},
		},

		// ── Refactoring cluster ───────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeSkill,
			Name:     "code-clean-code",
			Title:    "Clean Code Skill",
			Category: "refactoring",
			Desc:     "SOLID, DRY, and clean code refactoring skill",
			Keywords: map[string]int{"solid": 3, "dry": 2, "refactor": 2, "clean": 1},
		},
		index.Entry{
			Type:     index.TypeCommand,
			Name:     "dc-refactor-cmd",
			Title:    "Refactor Command",
			Category: "refactoring",
			Desc:     "Run refactor audit and lint on codebase",
			Keywords: map[string]int{"refactor": 2, "audit": 2, "lint": 2},
		},

		// ── Testing ──────────────────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeSkill,
			Name:     "code-testing-qa",
			Title:    "Testing and QA Skill",
			Category: "testing",
			Desc:     "TDD, coverage, and mock-based testing skill",
			Keywords: map[string]int{"tdd": 3, "coverage": 2, "mock": 2, "quality": 1},
		},

		// ── Bash ──────────────────────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeKB,
			Name:     "bash-idioms",
			Title:    "Bash Script Idioms",
			Category: "bash",
			Desc:     "Common bash scripting idioms and trap patterns",
			Keywords: map[string]int{"bash": 3, "trap": 2, "script": 2, "errexit": 1},
		},

		// ── LLM cluster ──────────────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeKB,
			Name:     "openai-functions",
			Title:    "OpenAI Function Calling",
			Category: "openai",
			Desc:     "OpenAI function calling schema and usage",
			Keywords: map[string]int{"openai": 3, "function": 2, "schema": 1},
		},
		index.Entry{
			Type:     index.TypeKB,
			Name:     "llm-prompting",
			Title:    "LLM Prompting Techniques",
			Category: "llm",
			Desc:     "Prompt engineering and LLM instruction techniques",
			Keywords: map[string]int{"prompt": 3, "llm": 2, "instruction": 2},
		},

		// ── Commands ──────────────────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeCommand,
			Name:     "dc-commit-pr",
			Title:    "Commit and PR Command",
			Category: "git",
			Desc:     "Create commit and pull request via gh CLI",
			Keywords: map[string]int{"commit": 3, "pull": 2, "git": 2},
		},

		// ── Low-confidence fixture ────────────────────────────────────────────
		// "vampire" appears only here, weight=1. Score for "tell me a vampire joke"
		// lands at threshold (2) — below the low-confidence gate (threshold×2 = 4).
		index.Entry{
			Type:     index.TypeKB,
			Name:     "weak-signal-entry",
			Title:    "Weak Signal Entry",
			Category: "misc",
			Desc:     "Entry designed to produce a weak match signal",
			Keywords: map[string]int{"vampire": 1, "dark": 1, "night": 1},
		},

		// ── Context patterns ──────────────────────────────────────────────────
		index.Entry{
			Type:     index.TypeKB,
			Name:     "go-context-patterns",
			Title:    "Go Context Patterns",
			Category: "go",
			Desc:     "Context cancellation, deadline, and timeout patterns",
			Keywords: map[string]int{"deadline": 3, "timeout": 2, "cancellation": 2},
		},
	)
}
