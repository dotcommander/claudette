package search

// categoryAliases maps alternative names to canonical category names.
// Used to boost scoring when a prompt token matches a category synonym.
var categoryAliases = map[string]string{
	"golang":     "go",
	"goroutine":  "go",
	"goroutines": "go",
	"gpt":        "openai",
	"gpt4":       "openai",
	"gpt5":       "openai",
	"chatgpt":    "openai",
	"claude":     "claude-code",
	"claudecode": "claude-code",
	"refactor":   "refactoring",
	"smell":      "refactoring",
	"smells":     "refactoring",
	"pig":        "piglet",
	"extension":  "piglet",
	"extensions": "piglet",
	"sdk":        "piglet",
	"shell":      "bash",
	"script":     "bash",
	"scripting":  "bash",
	"zsh":        "bash",
}

// CategoryAlias returns the canonical category for a token alias, if one exists.
func CategoryAlias(token string) (string, bool) {
	cat, ok := categoryAliases[token]
	return cat, ok
}
