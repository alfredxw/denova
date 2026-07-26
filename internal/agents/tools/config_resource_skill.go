package tools

import (
	"context"
	"fmt"
	"path"
	"strings"

	"denova/config"
	novaskills "denova/internal/agents/skills"
	"denova/internal/configresources"
)

type skillConfigValue struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Agents      []string `json:"agents,omitempty"`
	Content     string   `json:"content,omitempty"`
}

func newSkillConfigResource(cfg *config.Config) configresources.Adapter {
	dirs := skillDirs(cfg)
	return configResourceAdapter{
		descriptor: configresources.Descriptor{
			Name: "skill", Description: "Denova Skills stored in user or workspace scope.",
			Scopes: []string{string(novaskills.ScopeUser), string(novaskills.ScopeWorkspace)}, Operations: configCRUDOperations(),
			RevisionField: "revision", Reference: "references/skill.md",
		},
		list: func(ctx context.Context, request configresources.ReadRequest) (any, error) {
			snapshot, err := novaskills.SnapshotFor(ctx, dirs)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(request.Scope) == "" {
				return snapshot, nil
			}
			scope := novaskills.Scope(request.Scope)
			filtered := snapshot.Skills[:0]
			for _, item := range snapshot.Skills {
				if item.Scope == scope {
					filtered = append(filtered, item)
				}
			}
			snapshot.Skills = filtered
			return snapshot, nil
		},
		get: func(ctx context.Context, request configresources.ReadRequest) (any, error) {
			scope, err := editableSkillScope(request.Scope)
			if err != nil {
				return nil, err
			}
			result := make([]any, 0, len(request.IDs))
			for _, rawID := range normalizeConfigIDs(request.IDs) {
				id, parseErr := parseSkillResourceID(rawID)
				if parseErr != nil {
					return nil, parseErr
				}
				if id.Reference != "" {
					doc, readErr := novaskills.ReadSkillFile(ctx, dirs, scope, id.Name, id.Reference)
					if readErr != nil {
						return nil, readErr
					}
					result = append(result, doc)
					continue
				}
				doc, readErr := novaskills.ReadDocument(ctx, dirs, scope, id.Name)
				if readErr != nil {
					return nil, readErr
				}
				result = append(result, doc)
			}
			return result, nil
		},
		apply: func(ctx context.Context, mutation configresources.Mutation) (any, error) {
			scope, err := editableSkillScope(mutation.Scope)
			if err != nil {
				return nil, err
			}
			var value skillConfigValue
			switch mutation.Operation {
			case configresources.ApplyCreate:
				if err := decodeConfigValue(mutation.Value, &value); err != nil {
					return nil, err
				}
				id, parseErr := parseSkillResourceID(firstConfigNonEmpty(mutation.ID, value.Name))
				if parseErr != nil {
					return nil, parseErr
				}
				if id.Reference != "" {
					file, createErr := novaskills.CreateSkillFile(ctx, dirs, scope, id.Name, id.Reference, value.Content)
					return skillFileMutationReceipt(mutation, id.String(), file, createErr)
				}
				var doc novaskills.Document
				if strings.TrimSpace(value.Content) == "" {
					doc, err = novaskills.CreateDocument(ctx, dirs, scope, id.Name, value.Description, value.Agents...)
				} else {
					doc, err = novaskills.CreateDocumentWithContent(ctx, dirs, scope, id.Name, value.Content)
				}
				return skillMutationReceipt(mutation, doc, err)
			case configresources.ApplyUpdate:
				if err := decodeConfigValue(mutation.Value, &value); err != nil {
					return nil, err
				}
				id, parseErr := parseSkillResourceID(mutation.ID)
				if parseErr != nil {
					return nil, parseErr
				}
				if id.Reference != "" {
					file, saveErr := novaskills.SaveSkillFileIfRevision(ctx, dirs, scope, id.Name, id.Reference, value.Content, mutation.Revision)
					return skillFileMutationReceipt(mutation, id.String(), file, saveErr)
				}
				doc, saveErr := novaskills.SaveDocumentIfRevision(ctx, dirs, scope, id.Name, value.Content, mutation.Revision)
				return skillMutationReceipt(mutation, doc, saveErr)
			case configresources.ApplyDelete:
				id, parseErr := parseSkillResourceID(mutation.ID)
				if parseErr != nil {
					return nil, parseErr
				}
				if id.Reference != "" {
					deleted, deleteErr := novaskills.DeleteSkillFileIfRevision(ctx, dirs, scope, id.Name, id.Reference, mutation.Revision)
					if deleteErr != nil {
						return nil, deleteErr
					}
					return configMutationReceipt{
						Resource: mutation.Resource, Operation: mutation.Operation, ID: id.String(),
						Revision: deleted.Revision, Value: deleted,
					}, nil
				}
				doc, deleteErr := novaskills.DeleteDocumentIfRevision(ctx, dirs, scope, id.Name, mutation.Revision)
				if deleteErr != nil {
					return nil, deleteErr
				}
				return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: id.Name, Revision: doc.Revision}, nil
			default:
				return nil, fmt.Errorf("unsupported skill operation %q", mutation.Operation)
			}
		},
	}
}

type skillResourceID struct {
	Name      string
	Reference string
}

func parseSkillResourceID(value string) (skillResourceID, error) {
	value = strings.TrimSpace(value)
	name, reference, found := strings.Cut(value, "/")
	if err := novaskills.ValidateName(name); err != nil {
		return skillResourceID{}, err
	}
	if !found {
		return skillResourceID{Name: name}, nil
	}
	if strings.Contains(reference, "\\") {
		return skillResourceID{}, fmt.Errorf("skill reference id must use forward slashes")
	}
	cleaned := path.Clean(reference)
	if cleaned != reference || cleaned == "references" || !strings.HasPrefix(cleaned, "references/") {
		return skillResourceID{}, fmt.Errorf("skill reference id must be <skill>/references/<file>")
	}
	return skillResourceID{Name: name, Reference: cleaned}, nil
}

func (id skillResourceID) String() string {
	if id.Reference == "" {
		return id.Name
	}
	return id.Name + "/" + id.Reference
}

func editableSkillScope(value string) (novaskills.Scope, error) {
	scope := novaskills.Scope(strings.TrimSpace(value))
	if scope != novaskills.ScopeUser && scope != novaskills.ScopeWorkspace {
		return "", fmt.Errorf("skill scope must be user or workspace")
	}
	return scope, nil
}

func skillMutationReceipt(mutation configresources.Mutation, doc novaskills.Document, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return configMutationReceipt{
		Resource: mutation.Resource, Operation: mutation.Operation, ID: doc.Name,
		Revision: doc.Revision, Value: doc,
	}, nil
}

func skillFileMutationReceipt(mutation configresources.Mutation, id string, doc novaskills.FileDocument, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	return configMutationReceipt{
		Resource: mutation.Resource, Operation: mutation.Operation, ID: id,
		Revision: doc.Revision, Value: doc,
	}, nil
}

func skillDirs(cfg *config.Config) []novaskills.Directory {
	if cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace)
}
