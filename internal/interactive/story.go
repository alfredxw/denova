package interactive

import (
	"strings"
	"sync"

	"denova/internal/conversationjournal"
)

const schemaVersion = 1
const maxStoryLineBytes = 16 * 1024 * 1024
const defaultFirstStoryTitle = "新的开始"

// DefaultStoryReplyTargetChars is the default target length for one interactive story turn.
const DefaultStoryReplyTargetChars = 2000

const (
	DefaultStoryChoiceCount = 5
	MinStoryChoiceCount     = 2
	MaxStoryChoiceCount     = 10
)

const maxStoryOpeningTextRunes = 4000

const (
	StoryOpeningModeAI     = "ai"
	StoryOpeningModePreset = "preset"
	StoryOpeningModeCustom = "custom"
)

const (
	StoryImageModeManual   = "manual"
	StoryImageModeInterval = "interval"
)

// Store manages interactive story data inside a workspace.
type Store struct {
	root                   string
	novaDir                string
	mu                     sync.Mutex
	rewriteJSONL           func(string, []any) error
	appendStoryRecord      func(string, []byte) error
	lastStoryReplayByStory map[string]StoryJournalReplayStats
	heldStoryLeases        map[string]int
	storyJournals          map[string]*storyJournalHandle
}

type storyJournalHandle struct {
	journal    *conversationjournal.Journal
	projection *storyJournalProjection
	recent     map[string]storyRecentCache
}

type storyRecentCache struct {
	cursor  conversationjournal.Cursor
	meta    StoryMeta
	records []StoryEventRecord
}

// NewStore creates an interactive store rooted at the workspace directory.
func NewStore(root string) *Store {
	return &Store{root: root}
}

// NewStoreWithNovaDir creates an interactive store that can resolve reusable
// director modules from the workspace .denova directory.
func NewStoreWithNovaDir(root, novaDir string) *Store {
	return &Store{root: root, novaDir: strings.TrimSpace(novaDir)}
}

// Root returns the workspace root.
func (s *Store) Root() string {
	return s.root
}
