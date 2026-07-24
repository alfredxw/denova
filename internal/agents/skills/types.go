package skills

const (
	SkillFileName = "SKILL.md"

	ScopeBuiltin   Scope = "builtin"
	ScopeUser      Scope = "user"
	ScopeWorkspace Scope = "workspace"
)

// ContextMode controls whether a Skill executes in the current Agent context
// or in an isolated child context. Empty means inline instructions only.
type ContextMode string

const (
	ContextModeFork            ContextMode = "fork"
	ContextModeForkWithContext ContextMode = "fork_with_context"
)

// FrontMatter is the stable SKILL.md metadata understood by Denova. It lives
// in the product Skills module so the catalog is independent from any Agent
// framework implementation.
type FrontMatter struct {
	Name        string      `yaml:"name"`
	Description string      `yaml:"description"`
	Context     ContextMode `yaml:"context"`
	Agent       string      `yaml:"agent"`
	Model       string      `yaml:"model"`
}

// Skill is one resolved instruction bundle. BaseDirectory is the directory
// containing SKILL.md and is used to resolve bounded supporting files.
type Skill struct {
	FrontMatter
	Content       string
	BaseDirectory string
}

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
	Content  string      `json:"content"`
	Revision string      `json:"revision"`
	Files    []SkillFile `json:"files,omitempty"`
}

// FileDocument is a single supporting file payload inside a Skill directory.
type FileDocument struct {
	Skill    SkillSummary `json:"skill"`
	File     SkillFile    `json:"file"`
	Content  string       `json:"content"`
	Revision string       `json:"revision"`
}
