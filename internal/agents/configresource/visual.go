package configresource

import (
	"context"
	"fmt"
	"strings"

	imagepreset "denova/internal/image/preset"
	"denova/internal/style"
)

func newStyleReferenceResource(novaDir string) Adapter {
	lib := style.NewLibrary(novaDir)
	leasePath := configResourceLeasePath(novaDir, "style_reference")
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: "style_reference", Description: "Shared Markdown style references for writing and game modes.",
			Scopes: []string{"user"}, Operations: configCRUDOperations(), RevisionField: "revision", Reference: "references/style-reference.md",
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				items, err := lib.List()
				if err != nil {
					return nil, err
				}
				return NewCatalog(items), nil
			})
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				ids := normalizeConfigIDs(request.IDs)
				if len(ids) != 1 {
					return nil, fmt.Errorf("style_reference exact read requires one id")
				}
				return lib.Read(ids[0])
			})
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				return applyStyleReferenceMutation(lib, mutation)
			})
		},
	}
}

func applyStyleReferenceMutation(lib *style.Library, mutation Mutation) (any, error) {
	switch mutation.Operation {
	case ApplyCreate:
		var value style.WriteRequest
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
	case ApplyUpdate:
		var value struct {
			Content string `json:"content"`
		}
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		doc, err := lib.Update(style.UpdateRequest{Path: mutation.ID, Content: value.Content, BaseRevision: mutation.Revision})
		if err != nil {
			return nil, err
		}
		return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: doc.Reference.DisplayPath, Revision: doc.Revision, Value: doc}, nil
	case ApplyDelete:
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

func newImagePresetResource(novaDir string) Adapter {
	lib := imagepreset.NewLibrary(novaDir)
	leasePath := configResourceLeasePath(novaDir, "image_preset")
	return configResourceAdapter{
		descriptor: Descriptor{
			Name: "image_preset", Description: "Shared image generation presets for writing and game modes.",
			Scopes: []string{"user"}, Operations: configCRUDOperations(), RevisionField: "revision", Reference: "references/image-preset.md",
		},
		list: func(ctx context.Context, _ ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				items, err := lib.List()
				if err != nil {
					return nil, err
				}
				return NewCatalog(items), nil
			})
		},
		get: func(ctx context.Context, request ReadRequest) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				ids := normalizeConfigIDs(request.IDs)
				if len(ids) != 1 {
					return nil, fmt.Errorf("image_preset exact read requires one id")
				}
				return lib.Get(ids[0])
			})
		},
		apply: func(ctx context.Context, mutation Mutation) (any, error) {
			return withConfigResourceLease(ctx, leasePath, func() (any, error) {
				return applyImagePresetMutation(lib, mutation)
			})
		},
	}
}

func applyImagePresetMutation(lib *imagepreset.Library, mutation Mutation) (any, error) {
	var value imagepreset.Preset
	switch mutation.Operation {
	case ApplyCreate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := lib.Create(value)
		return configPresetReceipt(mutation, item, err)
	case ApplyUpdate:
		if err := decodeConfigValue(mutation.Value, &value); err != nil {
			return nil, err
		}
		item, err := lib.Update(mutation.ID, value, mutation.Revision)
		return configPresetReceipt(mutation, item, err)
	case ApplyDelete:
		current, err := lib.Get(mutation.ID)
		if err != nil {
			return nil, err
		}
		if err := requireConfigRevision(mutation.Resource, mutation.ID, mutation.Revision, current.Revision); err != nil {
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

func configPresetReceipt(mutation Mutation, item imagepreset.Preset, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(item.ID)
	return configMutationReceipt{Resource: mutation.Resource, Operation: mutation.Operation, ID: id, Revision: item.Revision, Value: item}, nil
}

func configCRUDOperations() []string {
	return []string{ReadList, ReadGet, ApplyCreate, ApplyUpdate, ApplyDelete}
}
