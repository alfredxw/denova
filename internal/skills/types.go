package skills

const (
	SkillFileName = "SKILL.md"

	ScopeBuiltin   Scope = "builtin"
	ScopeUser      Scope = "user"
	ScopeWorkspace Scope = "workspace"
)

// Scope identifies where a skill definition is stored.
type Scope string

// Directory is a scanned skill root. Later directories override earlier ones.
type Directory struct {
	Scope    Scope  `json:"scope"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}

// ScopeInfo is returned to the frontend for displaying editable locations.
type ScopeInfo struct {
	Scope    Scope  `json:"scope"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}

// SkillSummary describes a discovered skill.
type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Scope       Scope  `json:"scope"`
	Path        string `json:"path"`
	Editable    bool   `json:"editable"`
	Active      bool   `json:"active"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// SkillFile describes a regular file stored inside a Skill directory.
type SkillFile struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Entry     bool   `json:"entry"`
	Editable  bool   `json:"editable"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// Snapshot is the full skills management view returned by the API.
type Snapshot struct {
	Scopes []ScopeInfo    `json:"scopes"`
	Skills []SkillSummary `json:"skills"`
}

// Document is a single editable SKILL.md payload.
type Document struct {
	SkillSummary
	Content string      `json:"content"`
	Files   []SkillFile `json:"files,omitempty"`
}

// FileDocument is a single supporting file payload inside a Skill directory.
type FileDocument struct {
	Skill   SkillSummary `json:"skill"`
	File    SkillFile    `json:"file"`
	Content string       `json:"content"`
}
