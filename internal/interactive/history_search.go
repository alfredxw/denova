package interactive

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultStoryHistorySearchLimit = 8
	MaxStoryHistorySearchLimit     = 12
	storyHistoryUserExcerptRunes   = 320
	storyHistoryNarrativeRunes     = 1200
	storyHistoryStateSummaryRunes  = 480
)

// StoryHistorySearchRequest describes a bounded lookup over committed Turn
// events. The result is a rebuildable projection; TurnEvent remains the only
// historical source of truth.
type StoryHistorySearchRequest struct {
	Keywords     []string `json:"keywords,omitempty"`
	Match        string   `json:"match,omitempty"`
	BeforeTurnID string   `json:"before_turn_id,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// StoryHistoryHit carries the exact Turn source ID so callers can distinguish
// historical evidence from current state and future planning.
type StoryHistoryHit struct {
	TurnID       string   `json:"turn_id"`
	BranchID     string   `json:"branch_id"`
	Timestamp    string   `json:"timestamp"`
	UserAction   string   `json:"user_action"`
	Narrative    string   `json:"narrative"`
	StateChanges []string `json:"state_changes,omitempty"`
	Score        int      `json:"score,omitempty"`
}

type StoryHistorySearchResult struct {
	StoryID      string            `json:"story_id"`
	BranchID     string            `json:"branch_id"`
	Keywords     []string          `json:"keywords,omitempty"`
	Match        string            `json:"match"`
	Limit        int               `json:"limit"`
	ScannedTurns int               `json:"scanned_turns"`
	Truncated    bool              `json:"truncated"`
	Hits         []StoryHistoryHit `json:"hits"`
}

type scoredStoryHistoryHit struct {
	hit   StoryHistoryHit
	index int
}

// SearchStoryHistory searches committed turns on one resolved branch path.
// Empty keywords intentionally return the most recent turns, which also makes
// the same tool useful as a bounded history browser.
func (s *Store) SearchStoryHistory(storyID, branchID string, req StoryHistorySearchRequest) (StoryHistorySearchResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	keywords := normalizeStoryHistoryKeywords(req.Keywords)
	match := normalizeStoryHistoryMatch(req.Match)
	limit := normalizeStoryHistoryLimit(req.Limit)
	beforeTurnID := strings.TrimSpace(req.BeforeTurnID)
	var (
		cursor         string
		resolvedBranch string
		allTop         = make([]scoredStoryHistoryHit, 0, limit)
		beforeTop      = make([]scoredStoryHistoryHit, 0, limit)
		allScanned     int
		beforeScanned  int
		allMatches     int
		beforeMatches  int
		newestIndex    int
		foundBefore    = beforeTurnID == ""
	)
	for {
		loaded, err := s.readStoryHistoryPageLocked(storyID, branchID, cursor, maxStoryHistoryPageTurns, true)
		if err != nil {
			return StoryHistorySearchResult{}, err
		}
		if resolvedBranch == "" {
			resolvedBranch = loaded.page.BranchID
		}
		// Pages are chronological; walk each one backwards so tie-breaking and
		// before_turn_id can be evaluated while streaming newest to oldest.
		for index := len(loaded.page.Turns) - 1; index >= 0; index-- {
			turn := loaded.page.Turns[index]
			position := -newestIndex
			newestIndex++
			allScanned++
			allTop, allMatches = retainStoryHistorySearchHit(allTop, allMatches, turn, keywords, match, position, limit)
			if beforeTurnID != "" && turn.ID == beforeTurnID {
				foundBefore = true
				continue
			}
			if !foundBefore {
				continue
			}
			beforeScanned++
			beforeTop, beforeMatches = retainStoryHistorySearchHit(beforeTop, beforeMatches, turn, keywords, match, position, limit)
		}
		if !loaded.page.HasMore || strings.TrimSpace(loaded.page.BeforeCursor) == "" {
			break
		}
		cursor = loaded.page.BeforeCursor
	}

	scored, scannedTurns, matchedTurns := beforeTop, beforeScanned, beforeMatches
	if beforeTurnID != "" && !foundBefore {
		// Preserve legacy behavior for an unknown boundary: search the complete
		// branch instead of silently returning an empty result.
		scored, scannedTurns, matchedTurns = allTop, allScanned, allMatches
	}
	sortStoryHistoryHits(scored)
	hits := make([]StoryHistoryHit, 0, len(scored))
	for _, item := range scored {
		hits = append(hits, item.hit)
	}
	return StoryHistorySearchResult{
		StoryID:      storyID,
		BranchID:     resolvedBranch,
		Keywords:     keywords,
		Match:        match,
		Limit:        limit,
		ScannedTurns: scannedTurns,
		Truncated:    matchedTurns > limit,
		Hits:         hits,
	}, nil
}

func retainStoryHistorySearchHit(
	top []scoredStoryHistoryHit,
	matchedCount int,
	turn TurnEvent,
	keywords []string,
	match string,
	position int,
	limit int,
) ([]scoredStoryHistoryHit, int) {
	score, matched := storyHistoryMatchScore(turn, keywords, match)
	if !matched {
		return top, matchedCount
	}
	matchedCount++
	top = append(top, scoredStoryHistoryHit{
		index: position,
		hit: StoryHistoryHit{
			TurnID:       turn.ID,
			BranchID:     turn.BranchID,
			Timestamp:    turn.Ts,
			UserAction:   boundedStoryHistoryText(turn.User, storyHistoryUserExcerptRunes),
			Narrative:    boundedStoryHistoryText(turn.Narrative, storyHistoryNarrativeRunes),
			StateChanges: storyHistoryStateChanges(turn.StateDelta),
			Score:        score,
		},
	})
	sortStoryHistoryHits(top)
	if len(top) > limit {
		top = top[:limit]
	}
	return top, matchedCount
}

func sortStoryHistoryHits(values []scoredStoryHistoryHit) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].hit.Score != values[j].hit.Score {
			return values[i].hit.Score > values[j].hit.Score
		}
		return values[i].index > values[j].index
	})
}

func normalizeStoryHistoryKeywords(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeStoryHistoryText(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) == 8 {
			break
		}
	}
	return result
}

func normalizeStoryHistoryMatch(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "all") {
		return "all"
	}
	return "any"
}

func normalizeStoryHistoryLimit(value int) int {
	if value <= 0 {
		return DefaultStoryHistorySearchLimit
	}
	if value > MaxStoryHistorySearchLimit {
		return MaxStoryHistorySearchLimit
	}
	return value
}

func storyHistoryMatchScore(turn TurnEvent, keywords []string, match string) (int, bool) {
	if len(keywords) == 0 {
		return 1, true
	}
	user := normalizeStoryHistoryText(turn.User)
	narrative := normalizeStoryHistoryText(turn.Narrative)
	state := normalizeStoryHistoryText(strings.Join(storyHistoryStateChanges(turn.StateDelta), " "))
	matched := 0
	score := 0
	for _, keyword := range keywords {
		keywordScore := 0
		if strings.Contains(user, keyword) {
			keywordScore += 5
		}
		if strings.Contains(narrative, keyword) {
			keywordScore += 3
		}
		if strings.Contains(state, keyword) {
			keywordScore += 4
		}
		if keywordScore > 0 {
			matched++
			score += keywordScore
		}
	}
	if match == "all" {
		return score, matched == len(keywords)
	}
	return score, matched > 0
}

func normalizeStoryHistoryText(value string) string {
	value = cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
	return strings.Join(strings.FieldsFunc(value, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	}), " ")
}

func storyHistoryStateChanges(delta *StateDelta) []string {
	if delta == nil {
		return nil
	}
	changes := make([]string, 0, len(delta.Ops)+len(delta.ActorOps))
	for _, op := range delta.Ops {
		if lifecycle, ok := storyHistoryActorLifecycleChange(op); ok {
			changes = append(changes, boundedStoryHistoryText(lifecycle, storyHistoryStateSummaryRunes))
			continue
		}
		changes = append(changes, boundedStoryHistoryText(op.Op+" "+op.Path+" "+storyHistoryValue(op.Value)+" "+op.Reason, storyHistoryStateSummaryRunes))
	}
	for _, op := range delta.ActorOps {
		changes = append(changes, boundedStoryHistoryText(op.Op+" /"+op.ActorID+"/"+op.FieldID+" "+storyHistoryValue(op.Value)+" "+op.Reason, storyHistoryStateSummaryRunes))
	}
	if len(changes) > 12 {
		changes = append(changes[:12], "…")
	}
	return changes
}

func storyHistoryActorLifecycleChange(op StateOp) (string, bool) {
	prefix := actorArchiveRoot + "."
	if !strings.HasPrefix(op.Path, prefix) {
		return "", false
	}
	actorID := strings.TrimPrefix(op.Path, prefix)
	if normalizeStatePanelActorID(actorID) == "" {
		return "", false
	}
	switch strings.TrimSpace(op.Op) {
	case "set":
		return "archive /" + actorID + " " + op.Reason, true
	case "unset":
		return "restore /" + actorID + " " + op.Reason, true
	default:
		return "", false
	}
}

func storyHistoryValue(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

func boundedStoryHistoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:maxRunes-1])) + "…"
}
