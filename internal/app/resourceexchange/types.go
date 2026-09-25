package resourceexchange

import (
	"sync"
	"time"

	"denova/internal/app/resourcecatalog"
	"denova/internal/book/character"
	"denova/internal/platform"
	"denova/internal/project"
)

type Service struct {
	root     string
	registry *project.Registry
	catalog  *resourcecatalog.Service
	platform *platform.Manager
	mu       sync.Mutex
}

func New(root string, registry *project.Registry, catalog *resourcecatalog.Service, extensions *platform.Manager) *Service {
	return &Service{root: root, registry: registry, catalog: catalog, platform: extensions}
}

type PackageInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Author      string `json:"author,omitempty"`
	MinVersion  string `json:"min_denova_version,omitempty"`
}

type Resource struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Path     string   `json:"path"`
	Requires []string `json:"requires,omitempty"`
	Assets   []string `json:"assets,omitempty"`
}

type Manifest struct {
	Format        string      `json:"format"`
	SchemaVersion int         `json:"schema_version"`
	Package       PackageInfo `json:"package"`
	Resources     []Resource  `json:"resources"`
}

type PreviewResource struct {
	Resource
	Root        string              `json:"package_root"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Extension   *platform.Candidate `json:"extension,omitempty"`
	Digest      string              `json:"digest"`
}

type PackagePreview struct {
	ID        string            `json:"candidate_id"`
	Package   PackageInfo       `json:"package"`
	Format    string            `json:"format"`
	Resources []PreviewResource `json:"resources"`
}

type Preview struct {
	Character  *character.ImportResult `json:"character,omitempty"`
	ID         string                  `json:"preview_id"`
	Source     Source                  `json:"source"`
	ExpiresAt  time.Time               `json:"expires_at"`
	Candidates []PackagePreview        `json:"candidates"`
}

type LocalRef struct {
	Kind      string `json:"kind"`
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id,omitempty"`
	ID        string `json:"id"`
}

type Binding struct {
	UpstreamRemoved bool              `json:"upstream_removed,omitempty"`
	ResourceID      string            `json:"resource_id"`
	Local           LocalRef          `json:"local"`
	Ownership       string            `json:"ownership"`
	SourceDigest    string            `json:"source_digest"`
	Baseline        map[string]string `json:"baseline"`
}

type Installation struct {
	PendingPreviewID string      `json:"pending_preview_id,omitempty"`
	ID               string      `json:"installation_id"`
	Package          PackageInfo `json:"package"`
	Source           Source      `json:"source"`
	ProjectID        string      `json:"project_id,omitempty"`
	Tracking         string      `json:"tracking"`
	UpdateMode       string      `json:"update_mode"`
	Bindings         []Binding   `json:"bindings"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	CheckedAt        time.Time   `json:"checked_at,omitempty"`
	RemoteState      string      `json:"remote_state,omitempty"`
	LocalState       string      `json:"local_state,omitempty"`
}

type PlanRequest struct {
	automatic       bool
	PreviewID       string              `json:"preview_id"`
	CandidateID     string              `json:"candidate_id"`
	Resources       []string            `json:"resources"`
	ProjectID       string              `json:"project_id,omitempty"`
	SkillScope      string              `json:"skill_scope,omitempty"`
	UpdateMode      string              `json:"update_mode,omitempty"`
	InstallationID  string              `json:"installation_id,omitempty"`
	Grants          map[string][]string `json:"grants,omitempty"`
	Names           map[string]string   `json:"names,omitempty"`
	ReplaceModified bool                `json:"replace_modified,omitempty"`
}

type PlanItem struct {
	ResourceID string              `json:"resource_id"`
	Name       string              `json:"name"`
	Local      LocalRef            `json:"local"`
	Action     string              `json:"action"`
	Extension  *platform.Candidate `json:"extension,omitempty"`
	Grants     []string            `json:"grants,omitempty"`
}

type Plan struct {
	BackupID      string       `json:"backup_id,omitempty"`
	ID            string       `json:"plan_id"`
	PreviewID     string       `json:"preview_id"`
	CandidateID   string       `json:"candidate_id"`
	ExpiresAt     time.Time    `json:"expires_at"`
	Installation  Installation `json:"installation"`
	Items         []PlanItem   `json:"items"`
	PlatformState string       `json:"platform_state,omitempty"`
	Changes       []fileChange `json:"changes,omitempty"`
}

// PublicPlan excludes staged bytes and filesystem locations from API responses.
func (p Plan) PublicPlan() Plan { p.Changes = nil; return p }
