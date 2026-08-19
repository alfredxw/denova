package interactive

import (
	"crypto/sha256"
	interactivestate "denova/internal/interactive/state"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultStoryHistorySearchLimit = 8
	DefaultStoryHistoryResultBytes = 1024 * 1024
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
	Cursor       string   `json:"cursor,omitempty"`
	MaxBytes     int      `json:"-"`
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
	NextCursor   string            `json:"next_cursor,omitempty"`
	Hits         []StoryHistoryHit `json:"hits"`
}

type scoredStoryHistoryHit struct {
	hit   StoryHistoryHit
	index int
}

type storyHistorySearchCursor struct {
	Version     int    `json:"v"`
	Fingerprint string `json:"f"`
	HeadTurnID  string `json:"head"`
	AfterTurnID string `json:"after_turn"`
	AfterScore  int    `json:"after_score"`
	AfterIndex  int    `json:"after_index"`
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
	requestFingerprint := storyHistoryRequestFingerprint(storyID, branchID, keywords, match, beforeTurnID)
	pageCursor, err := decodeStoryHistorySearchCursor(req.Cursor, requestFingerprint)
	if err != nil {
		return StoryHistorySearchResult{}, err
	}
	if req.MaxBytes <= 0 {
		req.MaxBytes = DefaultStoryHistoryResultBytes
	}
	retainLimit := limit
	// Each hit has non-empty source fields. This is a memory bound derived
	// from the one shared result-byte budget, not a product item limit.
	maxBudgetedHits := req.MaxBytes/64 + 1
	if retainLimit > maxBudgetedHits {
		retainLimit = maxBudgetedHits
	}
	retainLimit++
	var (
		cursor         string
		resolvedBranch string
		allTop         = make([]scoredStoryHistoryHit, 0, retainLimit)
		beforeTop      = make([]scoredStoryHistoryHit, 0, retainLimit)
		allScanned     int
		beforeScanned  int
		allMatches     int
		beforeMatches  int
		newestIndex    int
		foundBefore    = beforeTurnID == ""
		withinSnapshot = pageCursor.HeadTurnID == ""
		foundHead      = withinSnapshot
		headTurnID     = pageCursor.HeadTurnID
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
			if !withinSnapshot {
				if turn.ID != pageCursor.HeadTurnID {
					continue
				}
				withinSnapshot = true
				foundHead = true
			}
			if headTurnID == "" {
				headTurnID = turn.ID
			}
			position := -newestIndex
			newestIndex++
			allScanned++
			allTop, allMatches = retainStoryHistorySearchHit(allTop, allMatches, turn, keywords, match, position, retainLimit, pageCursor)
			if beforeTurnID != "" && turn.ID == beforeTurnID {
				foundBefore = true
				continue
			}
			if !foundBefore {
				continue
			}
			beforeScanned++
			beforeTop, beforeMatches = retainStoryHistorySearchHit(beforeTop, beforeMatches, turn, keywords, match, position, retainLimit, pageCursor)
		}
		if !loaded.page.HasMore || strings.TrimSpace(loaded.page.BeforeCursor) == "" {
			break
		}
		cursor = loaded.page.BeforeCursor
	}
	if !foundHead {
		return StoryHistorySearchResult{}, errors.New("search_story_history cursor is stale because its history head no longer exists; search again")
	}

	scored, scannedTurns, matchedTurns := beforeTop, beforeScanned, beforeMatches
	if beforeTurnID != "" && !foundBefore {
		// Preserve legacy behavior for an unknown boundary: search the complete
		// branch instead of silently returning an empty result.
		scored, scannedTurns, matchedTurns = allTop, allScanned, allMatches
	}
	sortStoryHistoryHits(scored)
	result := StoryHistorySearchResult{
		StoryID:      storyID,
		BranchID:     resolvedBranch,
		Keywords:     keywords,
		Match:        match,
		Limit:        limit,
		ScannedTurns: scannedTurns,
		Hits:         []StoryHistoryHit{},
	}
	visibleLimit := min(limit, len(scored))
	for index := 0; index < visibleLimit; index++ {
		candidate := result
		candidate.Hits = append(append([]StoryHistoryHit(nil), result.Hits...), scored[index].hit)
		candidate.Truncated = matchedTurns > len(candidate.Hits)
		if candidate.Truncated {
			candidate.NextCursor, err = encodeStoryHistorySearchCursor(storyHistorySearchCursor{
				Version: 1, Fingerprint: requestFingerprint, HeadTurnID: headTurnID,
				AfterTurnID: scored[index].hit.TurnID, AfterScore: scored[index].hit.Score, AfterIndex: scored[index].index,
			})
			if err != nil {
				return StoryHistorySearchResult{}, fmt.Errorf("encode story history cursor: %w", err)
			}
		}
		encoded, encodeErr := json.Marshal(candidate)
		if encodeErr != nil {
			return StoryHistorySearchResult{}, encodeErr
		}
		if len(encoded) > req.MaxBytes {
			if len(result.Hits) == 0 {
				return StoryHistorySearchResult{}, fmt.Errorf("one story history hit exceeds the %d-byte shared result budget", req.MaxBytes)
			}
			break
		}
		result = candidate
	}
	result.Truncated = matchedTurns > len(result.Hits)
	if result.Truncated && len(result.Hits) > 0 {
		last := scored[len(result.Hits)-1]
		result.NextCursor, err = encodeStoryHistorySearchCursor(storyHistorySearchCursor{
			Version: 1, Fingerprint: requestFingerprint, HeadTurnID: headTurnID,
			AfterTurnID: last.hit.TurnID, AfterScore: last.hit.Score, AfterIndex: last.index,
		})
		if err != nil {
			return StoryHistorySearchResult{}, fmt.Errorf("encode story history cursor: %w", err)
		}
	}
	return result, nil
}

func retainStoryHistorySearchHit(
	top []scoredStoryHistoryHit,
	matchedCount int,
	turn TurnEvent,
	keywords []string,
	match string,
	position int,
	limit int,
	cursor storyHistorySearchCursor,
) ([]scoredStoryHistoryHit, int) {
	score, matched := storyHistoryMatchScore(turn, keywords, match)
	if !matched || !storyHistoryHitAfterCursor(score, position, cursor) {
		return top, matchedCount
	}
	matchedCount++
	userAction := turn.User
	if turn.UserContextOnly {
		userAction = ""
	}
	top = append(top, scoredStoryHistoryHit{
		index: position,
		hit: StoryHistoryHit{
			TurnID:       turn.ID,
			BranchID:     turn.BranchID,
			Timestamp:    turn.Ts,
			UserAction:   boundedStoryHistoryText(userAction, storyHistoryUserExcerptRunes),
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
	return value
}

func storyHistoryHitAfterCursor(score, index int, cursor storyHistorySearchCursor) bool {
	if cursor.AfterTurnID == "" {
		return true
	}
	if score != cursor.AfterScore {
		return score < cursor.AfterScore
	}
	return index < cursor.AfterIndex
}

func storyHistoryRequestFingerprint(storyID, branchID string, keywords []string, match, beforeTurnID string) string {
	payload := struct {
		StoryID      string   `json:"story_id"`
		BranchID     string   `json:"branch_id"`
		Keywords     []string `json:"keywords,omitempty"`
		Match        string   `json:"match"`
		BeforeTurnID string   `json:"before_turn_id,omitempty"`
	}{storyID, branchID, keywords, match, beforeTurnID}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
}

func encodeStoryHistorySearchCursor(cursor storyHistorySearchCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeStoryHistorySearchCursor(value, fingerprint string) (storyHistorySearchCursor, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return storyHistorySearchCursor{Version: 1, Fingerprint: fingerprint}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return storyHistorySearchCursor{}, errors.New("search_story_history cursor is invalid")
	}
	var cursor storyHistorySearchCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || cursor.HeadTurnID == "" || cursor.AfterTurnID == "" {
		return storyHistorySearchCursor{}, errors.New("search_story_history cursor is invalid")
	}
	if cursor.Fingerprint != fingerprint {
		return storyHistorySearchCursor{}, errors.New("search_story_history cursor does not belong to this query")
	}
	return cursor, nil
}

func storyHistoryMatchScore(turn TurnEvent, keywords []string, match string) (int, bool) {
	if len(keywords) == 0 {
		return 1, true
	}
	user := ""
	if !turn.UserContextOnly {
		user = normalizeStoryHistoryText(turn.User)
	}
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
	return changes
}

func storyHistoryActorLifecycleChange(op interactivestate.Op) (string, bool) {
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
