package tools

import (
	"context"
	"fmt"
	"strings"

	"denova/internal/configresources"
	"denova/internal/imagepreset"
	"denova/internal/styleref"
)

func newStyleReferenceResource(novaDir string) configresources.Adapter {
	lib := styleref.NewLibrary(novaDir)
	leasePath := configResourceLeasePath(novaDir, "style_reference")
	return configResourceAdapter{
		descriptor: configresources.Descriptor{
			Name: "style_reference", Description: "Shared Markdown style references for writing and game modes.",
			Scopes: []string{"user"}, Operations: configCRUDOperations(), RevisionField: "revision", Reference: "references/style-reference.md",
		},
		list: func(ctx context.Context, _ configresources.ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				items, err := lib.List()
				if err != nil {
					return nil, err
				}
				return configresources.NewCatalog(items), nil
			})
		},
		get: func(ctx context.Context, request configresources.ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				ids := normalizeConfigIDs(request.IDs)
				if len(ids) != 1 {
					return nil, fmt.Errorf("style_reference exact read requires one id")
				}
				return lib.Read(ids[0])
			})
		},
		apply: func(ctx context.Context, mutation configresources.Mutation) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				return applyStyleReferenceMutation(lib, mutation)
			})
		},
	}
}

func applyStyleReferenceMutation(lib *styleref.Library, mutation configresources.Mutation) (any, error) {
	switch mutation.Operation {
	case configresources.ApplyCreate:
		var value styleref.WriteRequest
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		created, err := lib.Create(value)
		if err != nil {
			return nil, err
		}
		doc, err := lib.Read(created.DisplayPath)
		if err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: created.DisplayPath, Revision: doc.Revision, Value: doc}, nil
	case configresources.ApplyUpdate:
		var value struct {
			Content string `json:"content"`
		}
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		doc, err := lib.Update(styleref.UpdateRequest{Path: mutation.ID, Content: value.Content, BaseRevision: mutation.Revision})
		if err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: doc.Reference.DisplayPath, Revision: doc.Revision, Value: doc}, nil
	case configresources.ApplyDelete:
		doc, err := lib.Read(mutation.ID)
		if err != nil {
			return nil, err
		}
		if err := requireConfigRevision(mutation.Resource, mutation.ID, mutation.Revision, doc.Revision); err != nil {
			return nil, err
		}
		if err := lib.Delete(mutation.ID); err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: mutation.ID}, nil
	default:
		return nil, fmt.Errorf("unsupported style reference operation %q", mutation.Operation)
	}
}

func newImagePresetResource(novaDir string) configresources.Adapter {
	lib := imagepreset.NewLibrary(novaDir)
	leasePath := configResourceLeasePath(novaDir, "image_preset")
	return configResourceAdapter{
		descriptor: configresources.Descriptor{
			Name: "image_preset", Description: "Shared image generation presets for writing and game modes.",
			Scopes: []string{"user"}, Operations: configCRUDOperations(), RevisionField: "updated_at", Reference: "references/image-preset.md",
		},
		list: func(ctx context.Context, _ configresources.ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				items, err := lib.List()
				if err != nil {
					return nil, err
				}
				return configresources.NewCatalog(items), nil
			})
		},
		get: func(ctx context.Context, request configresources.ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				ids := normalizeConfigIDs(request.IDs)
				if len(ids) != 1 {
					return nil, fmt.Errorf("image_preset exact read requires one id")
				}
				return lib.Get(ids[0])
			})
		},
		apply: func(ctx context.Context, mutation configresources.Mutation) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				return applyImagePresetMutation(lib, mutation)
			})
		},
	}
}

func applyImagePresetMutation(lib *imagepreset.Library, mutation configresources.Mutation) (any, error) {
	var value imagepreset.Preset
	switch mutation.Operation {
	case configresources.ApplyCreate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := lib.Create(value)
		return configPresetReceipt(mutation, item, err)
	case configresources.ApplyUpdate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := lib.Update(mutation.ID, value, mutation.Revision)
		return configPresetReceipt(mutation, item, err)
	case configresources.ApplyDelete:
		current, err := lib.Get(mutation.ID)
		if err != nil {
			return nil, err
		}
		if err := requireConfigRevision(mutation.Resource, mutation.ID, mutation.Revision, current.UpdatedAt); err != nil {
			return nil, err
		}
		if err := lib.Delete(mutation.ID); err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: mutation.ID}, nil
	default:
		return nil, fmt.Errorf("unsupported image preset operation %q", mutation.Operation)
	}
}

func configPresetReceipt(mutation configresources.Mutation, item imagepreset.Preset, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(item.ID)
	return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: id, Revision: item.UpdatedAt, Value: item}, nil
}

func configCRUDOperations() []string {
	return []string{configresources.ReadList, configresources.ReadGet, configresources.ApplyCreate, configresources.ApplyUpdate, configresources.ApplyDelete}
}
