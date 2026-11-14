package resourceexchange

import (
	"sync"
	"time"

	"denova/config"
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

// PackageInfo is the single author-maintained identity and presentation record.
// Text fields use Locale; Translations contains other languages only. Market
// discovery derives resource kinds and source dates instead of duplicating them.
type PackageInfo struct {
	ID            string                        `json:"id"`
	Name          string                        `json:"name"`
	Description   string                        `json:"description,omitempty"`
	Version       string                        `json:"version,omitempty"`
	Author        string                        `json:"author,omitempty"`
	MinVersion    string                        `json:"min_denova_version,omitempty"`
	Locale        string                        `json:"locale,omitempty"`
	Translations  map[string]PackageTranslation `json:"translations,omitempty"`
	Tags          []string                      `json:"tags,omitempty"`
	Cover         string                        `json:"cover,omitempty"`
	Usage         string                        `json:"usage,omitempty"`
	Compatibility string                        `json:"compatibility,omitempty"`
	UpdatedAt     string                        `json:"updated_at,omitempty"`
}

type PackageTranslation struct {
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	Usage         string `json:"usage,omitempty"`
	Compatibility string `json:"compatibility,omitempty"`
}

type Resource struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Path     string   `json:"path"`
	Requires []string `json:"requires,omitempty"`
	Assets   []string `json:"assets,omitempty"`
}

type Manifest struct {
	Format        string               `json:"format"`
	SchemaVersion int                  `json:"schema_version"`
	Package       PackageInfo          `json:"package"`
	Resources     []Resource           `json:"resources"`
	GameDefaults  *PackageGameDefaults `json:"game_defaults,omitempty"`
}

type PreviewResource struct {
	ItemCount int `json:"item_count,omitempty"`
	Resource
	Root        string              `json:"package_root"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Extension   *platform.Candidate `json:"extension,omitempty"`
	Digest      string              `json:"digest"`
}

type PackagePreview struct {
	ID           string               `json:"candidate_id"`
	Package      PackageInfo          `json:"package"`
	Format       string               `json:"format"`
	Resources    []PreviewResource    `json:"resources"`
	GameDefaults *PackageGameDefaults `json:"game_defaults,omitempty"`
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

// CollectionMember tracks a source item inside a collection without adding a second library.
// Digest is the last applied local content; SourceDigest is the last reviewed
// upstream content. Keeping a local edit advances only SourceDigest.
type CollectionMember struct {
	ID              string `json:"id"`
	Digest          string `json:"digest"`
	SourceDigest    string `json:"source_digest,omitempty"`
	UpstreamRemoved bool   `json:"upstream_removed,omitempty"`
}

type Binding struct {
	// AppliedRelease retains the extension baseline when a newer source is
	// acknowledged without replacing the user's current release.
	AppliedRelease  string                      `json:"applied_release,omitempty"`
	Requires        []string                    `json:"requires,omitempty"`
	Members         map[string]CollectionMember `json:"members,omitempty"`
	UpstreamRemoved bool                        `json:"upstream_removed,omitempty"`
	ResourceID      string                      `json:"resource_id"`
	Local           LocalRef                    `json:"local"`
	Ownership       string                      `json:"ownership"`
	SourceDigest    string                      `json:"source_digest"`
	Baseline        map[string]string           `json:"baseline"`
}

type Installation struct {
	// ReviewedSource describes the last reviewed package, including unselected
	// resources. It prevents repeated notices for deliberately omitted additions.
	ReviewedSource   string                       `json:"reviewed_source,omitempty"`
	GameDefaults     *config.GameCreationDefaults `json:"game_defaults,omitempty"`
	PendingPreviewID string                       `json:"pending_preview_id,omitempty"`
	ID               string                       `json:"installation_id"`
	Package          PackageInfo                  `json:"package"`
	Source           Source                       `json:"source"`
	ProjectID        string                       `json:"project_id,omitempty"`
	Tracking         string                       `json:"tracking"`
	UpdateMode       string                       `json:"update_mode"`
	Bindings         []Binding                    `json:"bindings"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`
	CheckedAt        time.Time                    `json:"checked_at,omitempty"`
	RemoteState      string                       `json:"remote_state,omitempty"`
	LocalState       string                       `json:"local_state,omitempty"`
}

type PlanRequest struct {
	// SharedResources selects reuse (default) or independent copies on new imports.
	// Extensions retain their fixed identity and cannot be copied.
	SharedResources string `json:"shared_resources,omitempty"`
	// Resolutions are explicit conflict choices, keyed by resource and member ID.
	// The empty member ID addresses an indivisible resource (e.g. a Skill).
	Resolutions        map[string]map[string]string `json:"resolutions,omitempty"`
	GameDefaultsFields []string                     `json:"game_defaults_fields,omitempty"`
	automatic          bool
	PreviewID          string              `json:"preview_id"`
	CandidateID        string              `json:"candidate_id"`
	Resources          []string            `json:"resources"`
	ProjectID          string              `json:"project_id,omitempty"`
	SkillScope         string              `json:"skill_scope,omitempty"`
	UpdateMode         string              `json:"update_mode,omitempty"`
	InstallationID     string              `json:"installation_id,omitempty"`
	Grants             map[string][]string `json:"grants,omitempty"`
	Names              map[string]string   `json:"names,omitempty"`
	ReplaceModified    bool                `json:"replace_modified,omitempty"`
	// OverrideByDetail enables detail-aware duplicate resolution for character
	// card imports: a same-name existing lore item is replaced only when the
	// incoming item is more detailed. Package and extension imports never set it,
	// keeping receipt-identity update semantics.
	OverrideByDetail bool `json:"override_by_detail,omitempty"`
	// OverrideChoices are explicit duplicate resolutions for character card
	// imports, keyed by the incoming lore entry name. "override" replaces the
	// existing same-name entry; "keep" skips the incoming duplicate. They take
	// precedence over detail-aware resolution and are never set by package or
	// extension imports.
	OverrideChoices map[string]string `json:"override_choices,omitempty"`
	// ConfirmReview binds a confirmed import to the duplicate entries the user
	// reviewed at preview time. Its Duplicates carry the local identity and
	// version captured then; the confirm request must match them or the
	// affected entries are reported as changed/unreviewed and kept local.
	ConfirmReview *ConfirmReview `json:"confirm_review,omitempty"`
}

// ConfirmReview is the confirmation payload of a character card import: the
// reviewed duplicates (with preview-time local ID and version) and the
// per-name decisions the user made about them.
type ConfirmReview struct {
	Duplicates []ImportDuplicate `json:"duplicates"`
	Choices    map[string]string `json:"choices"`
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
	Updates             []UpdateItem                 `json:"updates,omitempty"`
	SkillGuards         []Binding                    `json:"skill_guards,omitempty"`
	GameDefaultsBefore  *config.GameCreationDefaults `json:"game_defaults_before,omitempty"`
	GameDefaultsApplied *config.GameCreationDefaults `json:"game_defaults_applied,omitempty"`
	BackupID            string                       `json:"backup_id,omitempty"`
	ID                  string                       `json:"plan_id"`
	PreviewID           string                       `json:"preview_id"`
	CandidateID         string                       `json:"candidate_id"`
	ExpiresAt           time.Time                    `json:"expires_at"`
	Installation        Installation                 `json:"installation"`
	Items               []PlanItem                   `json:"items"`
	PlatformState       string                       `json:"platform_state,omitempty"`
	Changes             []fileChange                 `json:"changes,omitempty"`

	// ImportOverrides and ImportSkipped report detail-aware duplicate resolution
	// for character card imports: names that replaced a less-detailed existing
	// entry and names kept over a less-detailed incoming duplicate.
	ImportOverrides []string `json:"import_overrides,omitempty"`
	ImportSkipped   []string `json:"import_skipped,omitempty"`

	// ImportDuplicates lists same-name lore entries detected during a character
	// card import, with content digests and the automatic resolution suggestion,
	// so the UI can ask for confirmation before any replacement is applied.
	// Each duplicate also carries the local entry identity and version at plan
	// time, so a later confirmation can validate that nothing changed meanwhile.
	ImportDuplicates []ImportDuplicate `json:"import_duplicates,omitempty"`

	// ImportChanged and ImportUnreviewed report duplicates that a confirmed
	// import must not replace: the local entry was edited or deleted since the
	// reviewed preview (ImportChanged), or a same-name entry appeared that the
	// user never reviewed (ImportUnreviewed). Both keep local content untouched.
	ImportChanged   []string `json:"import_changed,omitempty"`
	ImportUnreviewed []string `json:"import_unreviewed,omitempty"`

	// Unavailable selections keep the review visible but prevent application
	// until resources are resolved or the user deselects those defaults.
	GameDefaultsUnavailable []string `json:"game_defaults_unavailable,omitempty"`
}

// ImportDuplicate reports a same-name lore entry detected during a character
// card import: the existing local entry against the incoming entry, with the
// backend's automatic resolution suggestion ("override" or "keep"). ExistingID
// and ExistingVersion bind the review to the exact local entry the user saw:
// confirmation only replaces an entry whose identity and digest still match.
type ImportDuplicate struct {
	Name            string `json:"name"`
	ExistingPreview string `json:"existing_preview"`
	IncomingPreview string `json:"incoming_preview"`
	Suggested       string `json:"suggested"`
	ExistingID      string `json:"existing_id"`
	ExistingVersion string `json:"existing_version"`
}

// PublicPlan excludes staged bytes and filesystem locations from API responses.
func (p Plan) PublicPlan() Plan { p.Changes = nil; p.SkillGuards = nil; return p }
