# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `claudette install` command — successor to `init` with explicit path-per-side-effect output (names `~/.claude/settings.json` and `~/.config/claudette/` directly)
- `claudette uninstall` command — removes every claudette-owned hook from `~/.claude/settings.json`, deletes `~/.config/claudette/`, and prints the exact `rm` command to remove the binary
- `--version` and `-v` root flags via cobra's built-in version handling
- Commit-hash + `-dirty` suffix in version output when built from a VCS checkout without a module version

### Changed
- `claudette init` is now an alias for `claudette install` (back-compat)
- Install output reports each step with its absolute path, ends with `Reverse with: claudette uninstall`

## [0.5.1] - 2026-04-11

### Fixed
- Corrected invalid `PostToolResult` hook event name to `PostToolUse`

## [0.5.0] - 2026-04-11

### Added
- `claudette init` command — wires hooks into Claude Code settings and builds the initial index
- Configuration system at `~/.config/claudette/config.json` for custom source directories
- MIT license

### Changed
- Rewrote README and setup guide for init-based workflow

## [0.4.0] - 2026-04-11

### Added
- Usage tracking with progressive disclosure — records surfaced entries to append-only log
- Conversational suppression — stop words filtered before scoring to reduce noise
- Single-token confidence floor — weak matches from a single keyword are suppressed
- Benchmark expanded to 197 prompts across 26 categories (vague, stopwords, noise)

## [0.3.1] - 2026-04-10

### Changed
- Improved .gitignore coverage for benchmark output files

## [0.3.0] - 2026-04-10

### Added
- BM25 term saturation model replacing TF-IDF scoring
- Weak match suppression below 2× confidence threshold
- Section-aware body content extraction from markdown
- Table-driven scorer tests covering all scoring paths
- 96-prompt hook quality benchmark

### Changed
- Scorer rewritten with IDF multipliers and stem matching
- Index schema upgraded to v2 with weighted keywords and bigrams

## [0.2.0] - 2026-04-09

### Added
- IDF multipliers for term frequency weighting
- Weighted keyword extraction with bigrams and trigger words
- Verbose stderr diagnostics with timing and per-skip reasons

### Changed
- Extracted `walkMdFiles` helper, fixed frontmatter title extraction

## [0.1.0] - 2026-04-08

### Added
- Initial release — knowledge and skill discovery for Claude Code
- Keyword-overlap scoring with category aliasing
- YAML frontmatter and markdown heading parsing
- Cached index with automatic staleness detection
- `UserPromptSubmit` hook mode for sub-50ms context injection
- CLI with `search`, `kb`, `skill`, `scan`, and `version` commands

[0.5.1]: https://github.com/dotcommander/claudette/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/dotcommander/claudette/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/dotcommander/claudette/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/dotcommander/claudette/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/dotcommander/claudette/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/dotcommander/claudette/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/dotcommander/claudette/releases/tag/v0.1.0
