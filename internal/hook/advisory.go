package hook

import (
	"fmt"
	"strings"
	"time"

	"github.com/dotcommander/claudette/internal/index"
)

// staleBuildThresholdDays is the age at which an index is considered stale.
const staleBuildThresholdDays = 30

// SuggestedAliasMinCount is the minimum number of entries with pending alias
// candidates before the advisory fires. Keeps noise low for small indexes.
const SuggestedAliasMinCount = 3

// HookStatus carries advisory signals gathered during a single hook run.
// All fields default to zero meaning "nothing to report". Checkers read only
// HookStatus — no I/O, no globals — enabling pure-function unit tests without fixtures.
type HookStatus struct {
	RebuildMs           int      // >0 when index was rebuilt this run
	MissCount           int      // >0 when token miss clustering is active (len of FrequentMissTokens)
	ZeroKwCount         int      // >0 when entries with zero keywords were indexed
	ColdCount           int      // >0 when cold (never-matched) entries were returned
	IndexAgeDays        int      // age of the index in days (0 when unknown or fresh)
	FrequentMissTokens  []string // top missed tokens above cluster threshold; nil when below
	SuggestedAliasCount int      // >0 when entries have alias candidates pending review
}

// AdvisoryChecker is a priority-ordered advisory rule. Check must be a pure
// function: read HookStatus, return (active, message). No I/O. No globals.
type AdvisoryChecker struct {
	Name  string
	Check func(HookStatus) (active bool, message string)
}

// advisoryStatusSegment returns " | key=val ..." for non-zero HookStatus
// fields, or "" when all fields are zero. Fields appear in fixed order:
// rebuild_ms, miss_count, zero_kw_count, cold_count, index_age_days, suggested_alias_count.
// index_age_days only appears when above staleBuildThresholdDays (preserving
// the byte-identical invariant for healthy/recent indexes).
// suggested_alias_count only appears when >= SuggestedAliasMinCount.
// When "" is returned the stderr line is byte-identical to the pre-change format.
func advisoryStatusSegment(status HookStatus) string {
	showIndexAge := status.IndexAgeDays > staleBuildThresholdDays
	showSuggestedAliases := status.SuggestedAliasCount >= SuggestedAliasMinCount
	if status.RebuildMs == 0 && status.MissCount == 0 &&
		status.ZeroKwCount == 0 && status.ColdCount == 0 &&
		!showIndexAge && !showSuggestedAliases {
		return ""
	}

	seg := " |"
	if status.RebuildMs != 0 {
		seg += fmt.Sprintf(" rebuild_ms=%d", status.RebuildMs)
	}
	if status.MissCount != 0 {
		seg += fmt.Sprintf(" miss_count=%d", status.MissCount)
	}
	if status.ZeroKwCount != 0 {
		seg += fmt.Sprintf(" zero_kw_count=%d", status.ZeroKwCount)
	}
	if status.ColdCount != 0 {
		seg += fmt.Sprintf(" cold_count=%d", status.ColdCount)
	}
	if showIndexAge {
		seg += fmt.Sprintf(" index_age_days=%d", status.IndexAgeDays)
	}
	if showSuggestedAliases {
		seg += fmt.Sprintf(" suggested_alias_count=%d", status.SuggestedAliasCount)
	}
	return seg
}

// populateAdvisoryFields is the single seam for wiring index-state signals
// into HookStatus. Reads from idx and now; checker functions remain pure
// (HookStatus-only, no I/O). Extended per task; future tasks add fields here.
func populateAdvisoryFields(hs *HookStatus, idx *index.Index, now time.Time) {
	hs.IndexAgeDays = int(now.Sub(idx.BuildTime).Hours() / 24)
	hs.ZeroKwCount = idx.ZeroKeywordCount
	hs.ColdCount = idx.ColdEntryCount
	hs.FrequentMissTokens = idx.FrequentMissTokens
	hs.MissCount = len(idx.FrequentMissTokens)
	hs.SuggestedAliasCount = idx.SuggestedAliasCount
}

// --- Checker stubs (all inactive until their respective tasks land) ---

// checkStaleBuild activates when the index has not been rebuilt for more than
// staleBuildThresholdDays. Pure function — reads only HookStatus, no I/O.
func checkStaleBuild(s HookStatus) (bool, string) {
	if s.IndexAgeDays > staleBuildThresholdDays {
		return true, fmt.Sprintf("index is %d days old — run `claudette update` to refresh", s.IndexAgeDays)
	}
	return false, ""
}

func checkZeroKeyword(s HookStatus) (bool, string) {
	if s.ZeroKwCount > 0 {
		return true, fmt.Sprintf("%d entries have zero keywords — check frontmatter for parse failures", s.ZeroKwCount)
	}
	return false, ""
}

func checkColdEntries(s HookStatus) (bool, string) {
	if s.ColdCount > 0 {
		return true, fmt.Sprintf("%d entries have zero hits and are >90 days old — review for relevance", s.ColdCount)
	}
	return false, ""
}

func checkMissCluster(s HookStatus) (bool, string) {
	if len(s.FrequentMissTokens) == 0 {
		return false, ""
	}
	return true, fmt.Sprintf("frequently unmatched tokens: %s — consider adding KB entries or aliases",
		strings.Join(s.FrequentMissTokens, ", "))
}

func checkSuggestedAliases(s HookStatus) (bool, string) {
	if s.SuggestedAliasCount >= SuggestedAliasMinCount {
		return true, fmt.Sprintf("%d entries have suggested aliases pending review", s.SuggestedAliasCount)
	}
	return false, ""
}
