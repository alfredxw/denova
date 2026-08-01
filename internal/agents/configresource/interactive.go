package configresource

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/interactive"
	"denova/internal/interactive/teller"
)

type jsonConfigResource[T any] struct {
	name        string
	description string
	reference   string
	leasePath   string
	list        func() ([]T, error)
	get         func(string) (T, error)
	create      func(T) (T, error)
	update      func(string, T, string) (T, error)
	delete      func(string) error
	id          func(T) string
	revision    func(T) string
}

func (resource jsonConfigResource[T]) adapter() Adapter {
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: resource.name, Description: resource.description, Scopes: []string{"user"},
			Operations: configCRUDOperations(), RevisionField: "updated_at", Reference: resource.reference,
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, resource.leasePath, func() (any, error) {
				items, err := resource.list()
				if err != nil {
					return nil, err
				}
				return NewCatalog(items), nil
			})
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, resource.leasePath, func() (any, error) {
				ids := normalizeConfigIDs(request.IDs)
				if len(ids) != 1 {
					return nil, fmt.Errorf("%s exact read requires one id", resource.name)
				}
				return resource.get(ids[0])
			})
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			return withConfigResourceLease(ctx, resource.leasePath, func() (any, error) {
				return resource.apply(mutation)
			})
		},
	}
}

// apply runs after the resource lease is acquired, so every library-level
// read/CAS/write or read/CAS/delete sequence is one indivisible operation for
// all config adapters targeting the same resource directory.
func (resource jsonConfigResource[T]) apply(mutation Mutation) (any, error) {
	var value T
	switch mutation.Operation {
	case ApplyCreate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := resource.create(value)
		return resource.receipt(mutation, item, err)
	case ApplyUpdate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := resource.update(mutation.ID, value, mutation.Revision)
		return resource.receipt(mutation, item, err)
	case ApplyDelete:
		current, err := resource.get(mutation.ID)
		if err != nil {
			return nil, err
		}
		if err := requireConfigRevision(resource.name, mutation.ID, mutation.Revision, resource.revision(current)); err != nil {
			return nil, err
		}
		if err := resource.delete(mutation.ID); err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: resource.name, Operation: mutation.Operation, ID: mutation.ID}, nil
	default:
		return nil, fmt.Errorf("unsupported %s operation %q", resource.name, mutation.Operation)
	}
}

func (resource jsonConfigResource[T]) receipt(mutation Mutation, item T, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return configMutationReceipt{
		Resource: resource.name, Operation: mutation.Operation,
		ID: strings.TrimSpace(resource.id(item)), Revision: strings.TrimSpace(resource.revision(item)), Value: item,
	}, nil
}

func newNarrativeStyleResource(novaDir string) Adapter {
	lib := teller.NewLibrary(novaDir)
	return jsonConfigResource[teller.Definition]{
		name: "narrative_style", description: "Narrative style with writing/game availability, prompt slots, style references, and context policy.", reference: "references/narrative-style.md",
		leasePath: configResourceLeasePath(novaDir, "narrative_style"),
		list:      lib.List, get: lib.Get, create: lib.Create,
		update: func(id string, value teller.Definition, revision string) (teller.Definition, error) {
			return lib.Update(id, value, revision)
		},
		delete: lib.Delete, id: func(value teller.Definition) string { return value.ID }, revision: func(value teller.Definition) string { return value.UpdatedAt },
	}.adapter()
}

func newStoryDirectorResource(novaDir string) Adapter {
	lib := interactive.NewStoryDirectorLibrary(novaDir)
	return jsonConfigResource[interactive.StoryDirector]{
		name: "story_director", description: "Game-mode story orchestration and reusable module references.", reference: "references/story-director.md",
		leasePath: configResourceLeasePath(novaDir, "story_director"),
		list:      lib.List, get: lib.Get, create: lib.Create, update: lib.Update, delete: lib.Delete,
		id: func(value interactive.StoryDirector) string { return value.ID }, revision: func(value interactive.StoryDirector) string { return value.UpdatedAt },
	}.adapter()
}

func newEventPackageResource(novaDir string) Adapter {
	lib := interactive.NewEventPackageLibrary(novaDir)
	return jsonConfigResource[interactive.EventPackageModule]{
		name: "event_package", description: "Game-mode package of reusable event cards.", reference: "references/event-package.md",
		leasePath: configResourceLeasePath(novaDir, "event_package"),
		list:      lib.List, get: lib.Get, create: lib.Create, update: lib.Update, delete: lib.Delete,
		id: func(value interactive.EventPackageModule) string { return value.ID }, revision: func(value interactive.EventPackageModule) string { return value.UpdatedAt },
	}.adapter()
}

func newRuleSystemResource(novaDir string) Adapter {
	lib := interactive.NewRuleSystemLibrary(novaDir)
	return jsonConfigResource[interactive.RuleSystemModule]{
		name: "rule_system", description: "Game-mode d20 check policy, guidance, and state bindings.", reference: "references/rule-system.md",
		leasePath: configResourceLeasePath(novaDir, "rule_system"),
		list:      lib.List, get: lib.Get, create: lib.Create, update: lib.Update, delete: lib.Delete,
		id: func(value interactive.RuleSystemModule) string { return value.ID }, revision: func(value interactive.RuleSystemModule) string { return value.UpdatedAt },
	}.adapter()
}

func newStateSystemResource(novaDir string) Adapter {
	lib := interactive.NewActorStateLibrary(novaDir)
	return jsonConfigResource[interactive.ActorStateModule]{
		name: "state_system", description: "Game-mode Actor state schemas, initial actors, and trait pools.", reference: "references/state-system.md",
		leasePath: configResourceLeasePath(novaDir, "state_system"),
		list:      lib.List, get: lib.Get, create: lib.Create, update: lib.Update, delete: lib.Delete,
		id: func(value interactive.ActorStateModule) string { return value.ID }, revision: func(value interactive.ActorStateModule) string { return value.UpdatedAt },
	}.adapter()
}
