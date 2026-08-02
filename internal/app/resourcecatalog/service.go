// Package resourcecatalog manages the reusable creator resources stored in the
// Denova data directory. Resources are global catalogs consumed by both writing
// and game modes; they are deliberately independent from the active workspace.
package resourcecatalog

import (
	"errors"
	"strings"

	novaskills "denova/internal/agents/skills"
	imagepreset "denova/internal/image/preset"
	"denova/internal/interactive"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

// ErrDataDirRequired means a catalog service was created without a durable
// Denova data directory.
var ErrDataDirRequired = errors.New("resource catalog data directory is required")

// SkillDirectorySource supplies the current user/workspace Skill search path.
// Unlike global catalogs, workspace Skills follow the foreground workspace, so
// the source is evaluated for every operation.
type SkillDirectorySource interface {
	SkillDirectories() []novaskills.Directory
}

// Service owns all reusable resource libraries that share the same durable
// data-directory lifecycle.
type Service struct {
	initErr         error
	tellers         *teller.Library
	directors       *interactive.StoryDirectorLibrary
	eventPackages   *interactive.EventPackageLibrary
	ruleSystems     *interactive.RuleSystemLibrary
	actorStates     *interactive.ActorStateLibrary
	imagePresets    *imagepreset.Library
	styleReferences *style.Library
	skillSource     SkillDirectorySource
}

// NewService binds the global resource catalogs to dataDir. An invalid data
// directory is reported by operations so construction remains easy to compose
// in application service graphs.
func NewService(dataDir string, skillSource SkillDirectorySource) *Service {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return &Service{initErr: ErrDataDirRequired, skillSource: skillSource}
	}
	return &Service{
		tellers:         teller.NewLibrary(dataDir),
		directors:       interactive.NewStoryDirectorLibrary(dataDir),
		eventPackages:   interactive.NewEventPackageLibrary(dataDir),
		ruleSystems:     interactive.NewRuleSystemLibrary(dataDir),
		actorStates:     interactive.NewActorStateLibrary(dataDir),
		imagePresets:    imagepreset.NewLibrary(dataDir),
		styleReferences: style.NewLibrary(dataDir),
		skillSource:     skillSource,
	}
}

func (s *Service) ready() error {
	if s == nil {
		return ErrDataDirRequired
	}
	return s.initErr
}
