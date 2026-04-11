package search

// categoryAliases maps alternative names to canonical category names.
// Used to boost scoring when a prompt token matches a category synonym.
// Categories correspond to KB parent directory names (e.g., kb/go/ -> category "go").
var categoryAliases = map[string]string{ //nolint:gochecknoglobals // immutable lookup table
	// Go
	"golang":     "go",
	"goroutine":  "go",
	"goroutines": "go",
	"gopher":     "go",

	// OpenAI
	"gpt":     "openai",
	"gpt4":    "openai",
	"gpt5":    "openai",
	"chatgpt": "openai",
	"dalle":   "openai",
	"whisper": "openai",

	// Claude
	"claude":     "claude-code",
	"claudecode": "claude-code",
	"anthropic":  "claude-code",

	// Refactoring
	"refactor":  "refactoring",
	"smell":     "refactoring",
	"smells":    "refactoring",
	"cleanup":   "refactoring",
	"tech-debt": "refactoring",

	// Piglet
	"pig":        "piglet",
	"extension":  "piglet",
	"extensions": "piglet",
	"sdk":        "piglet",

	// Bash/Shell
	"shell":     "bash",
	"script":    "bash",
	"scripting": "bash",
	"zsh":       "bash",
	"terminal":  "bash",

	// LLM
	"ai":         "llm",
	"llms":       "llm",
	"inference":  "llm",
	"embedding":  "llm",
	"embeddings": "llm",

	// Python
	"py":      "python",
	"pip":     "python",
	"pytest":  "python",
	"django":  "python",
	"flask":   "python",
	"fastapi": "python",

	// TypeScript
	"ts":  "typescript",
	"tsx": "typescript",

	// JavaScript
	"js":     "javascript",
	"jsx":    "javascript",
	"nodejs": "javascript",
	"deno":   "javascript",

	// Rust
	"cargo": "rust",
	"rustc": "rust",
	"tokio": "rust",

	// PHP
	"composer": "php",
	"laravel":  "php",
	"symfony":  "php",

	// Database
	"db":         "database",
	"sql":        "database",
	"postgres":   "database",
	"postgresql": "database",
	"mysql":      "database",
	"sqlite":     "database",
	"redis":      "database",
	"mongo":      "database",
	"mongodb":    "database",

	// Frontend
	"ui":      "frontend",
	"ux":      "frontend",
	"css":     "frontend",
	"html":    "frontend",
	"dom":     "frontend",
	"react":   "frontend",
	"vue":     "frontend",
	"svelte":  "frontend",
	"angular": "frontend",

	// API
	"rest":      "api",
	"graphql":   "api",
	"grpc":      "api",
	"endpoint":  "api",
	"endpoints": "api",

	// DevOps
	"docker":     "devops",
	"kubernetes": "devops",
	"k8s":        "devops",
	"terraform":  "devops",
	"ansible":    "devops",

	// Git
	"github":    "git",
	"gitlab":    "git",
	"bitbucket": "git",
}

// CategoryAlias returns the canonical category for a token alias, if one exists.
func CategoryAlias(token string) (string, bool) {
	cat, ok := categoryAliases[token]
	return cat, ok
}
