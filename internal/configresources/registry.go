// Package configresources provides the single routing seam used by the
// Config Manager agent. Product resources implement Adapter; the model only
// sees the stable config_read/config_apply protocol built on top of Registry.
package configresources

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	ReadDescribe = "describe"
	ReadList     = "list"
	ReadGet      = "get"

	ApplyCreate = "create"
	ApplyUpdate = "update"
	ApplyDelete = "delete"
)

// Descriptor is the bounded, model-readable contract for one resource type.
// Details that change frequently belong in the config-manager Skill reference,
// not in this routing type.
type Descriptor struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Scopes        []string `json:"scopes,omitempty"`
	Operations    []string `json:"operations"`
	RevisionField string   `json:"revision_field,omitempty"`
	Reference     string   `json:"reference,omitempty"`
}

// ReadRequest describes a metadata, catalog, or exact-item read. IDs are
// intentionally bounded by the tool layer before reaching an Adapter.
type ReadRequest struct {
	Operation string
	Resource  string
	IDs       []string
	Scope     string
	Query     string
}

// Mutation is one independently committed resource change. A single mutation
// avoids pretending that heterogeneous resource writes can be atomic.
type Mutation struct {
	Operation string
	Resource  string
	ID        string
	Scope     string
	Revision  string
	Value     map[string]any
}

// Adapter owns the storage and validation rules for exactly one resource.
// Returning application values rather than encoded strings keeps serialization
// and output limits at the tool boundary.
type Adapter interface {
	Descriptor() Descriptor
	List(context.Context, ReadRequest) (any, error)
	Get(context.Context, ReadRequest) (any, error)
	Apply(context.Context, Mutation) (any, error)
}

// Registry routes stable resource names to implementations. It is immutable
// after construction and therefore safe to share across concurrent reads.
type Registry struct {
	adapters    map[string]Adapter
	descriptors map[string]Descriptor
	names       []string
}

func New(adapters ...Adapter) (*Registry, error) {
	registry := &Registry{
		adapters:    make(map[string]Adapter, len(adapters)),
		descriptors: make(map[string]Descriptor, len(adapters)),
	}
	for _, adapter := range adapters {
		if adapter == nil {
			return nil, errors.New("config resource adapter is nil")
		}
		descriptor := normalizeDescriptor(adapter.Descriptor())
		if descriptor.Name == "" {
			return nil, errors.New("config resource adapter name is empty")
		}
		if _, exists := registry.adapters[descriptor.Name]; exists {
			return nil, fmt.Errorf("duplicate config resource adapter %q", descriptor.Name)
		}
		if err := validateDescriptor(descriptor); err != nil {
			return nil, err
		}
		registry.adapters[descriptor.Name] = adapter
		registry.descriptors[descriptor.Name] = descriptor
		registry.names = append(registry.names, descriptor.Name)
	}
	sort.Strings(registry.names)
	return registry, nil
}

func (r *Registry) Describe(resource string) ([]Descriptor, error) {
	if r == nil {
		return nil, errors.New("config resource registry is nil")
	}
	resource = strings.TrimSpace(resource)
	if resource != "" {
		if _, err := r.adapter(resource); err != nil {
			return nil, err
		}
		return []Descriptor{r.descriptors[resource]}, nil
	}
	result := make([]Descriptor, 0, len(r.names))
	for _, name := range r.names {
		result = append(result, r.descriptors[name])
	}
	return result, nil
}

func (r *Registry) Read(ctx context.Context, request ReadRequest) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request.Operation = strings.TrimSpace(request.Operation)
	if request.Operation == "" {
		request.Operation = ReadList
	}
	request.Resource = strings.TrimSpace(request.Resource)
	request.Scope = strings.TrimSpace(request.Scope)
	request.Query = strings.TrimSpace(request.Query)
	switch request.Operation {
	case ReadDescribe:
		return r.Describe(request.Resource)
	case ReadList:
		adapter, err := r.adapter(request.Resource)
		if err != nil {
			return nil, err
		}
		if err := r.validateRequest(request.Resource, request.Operation, request.Scope); err != nil {
			return nil, err
		}
		return adapter.List(ctx, request)
	case ReadGet:
		if len(request.IDs) == 0 {
			return nil, errors.New("config_read get requires at least one id")
		}
		adapter, err := r.adapter(request.Resource)
		if err != nil {
			return nil, err
		}
		if err := r.validateRequest(request.Resource, request.Operation, request.Scope); err != nil {
			return nil, err
		}
		return adapter.Get(ctx, request)
	default:
		return nil, fmt.Errorf("unsupported config read operation %q", request.Operation)
	}
}

func (r *Registry) Apply(ctx context.Context, mutation Mutation) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mutation.Operation = strings.TrimSpace(mutation.Operation)
	mutation.Resource = strings.TrimSpace(mutation.Resource)
	mutation.ID = strings.TrimSpace(mutation.ID)
	mutation.Scope = strings.TrimSpace(mutation.Scope)
	mutation.Revision = strings.TrimSpace(mutation.Revision)
	switch mutation.Operation {
	case ApplyCreate:
		if len(mutation.Value) == 0 {
			return nil, errors.New("config_apply create requires value")
		}
	case ApplyUpdate:
		if mutation.ID == "" {
			return nil, errors.New("config_apply update requires id")
		}
		if mutation.Revision == "" {
			return nil, errors.New("config_apply update requires revision from config_read")
		}
		if len(mutation.Value) == 0 {
			return nil, errors.New("config_apply update requires value")
		}
	case ApplyDelete:
		if mutation.ID == "" {
			return nil, errors.New("config_apply delete requires id")
		}
		if mutation.Revision == "" {
			return nil, errors.New("config_apply delete requires revision from config_read")
		}
	default:
		return nil, fmt.Errorf("unsupported config mutation operation %q", mutation.Operation)
	}
	adapter, err := r.adapter(mutation.Resource)
	if err != nil {
		return nil, err
	}
	if err := r.validateRequest(mutation.Resource, mutation.Operation, mutation.Scope); err != nil {
		return nil, err
	}
	return adapter.Apply(ctx, mutation)
}

func (r *Registry) validateRequest(resource, operation, scope string) error {
	descriptor, ok := r.descriptors[resource]
	if !ok {
		return fmt.Errorf("unknown config resource %q", resource)
	}
	if !containsString(descriptor.Operations, operation) {
		return fmt.Errorf("config resource %q does not support operation %q", resource, operation)
	}
	if scope != "" && len(descriptor.Scopes) != 0 && !containsString(descriptor.Scopes, scope) {
		return fmt.Errorf("config resource %q does not support scope %q; available: %s", resource, scope, strings.Join(descriptor.Scopes, ", "))
	}
	return nil
}

func (r *Registry) adapter(resource string) (Adapter, error) {
	if r == nil {
		return nil, errors.New("config resource registry is nil")
	}
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return nil, errors.New("config resource is required")
	}
	adapter, ok := r.adapters[resource]
	if !ok {
		return nil, fmt.Errorf("unknown config resource %q; available: %s", resource, strings.Join(r.names, ", "))
	}
	return adapter, nil
}

func normalizeDescriptor(descriptor Descriptor) Descriptor {
	descriptor.Name = strings.TrimSpace(descriptor.Name)
	descriptor.Description = strings.TrimSpace(descriptor.Description)
	descriptor.RevisionField = strings.TrimSpace(descriptor.RevisionField)
	descriptor.Reference = strings.TrimSpace(descriptor.Reference)
	descriptor.Scopes = compactStrings(descriptor.Scopes)
	descriptor.Operations = compactStrings(descriptor.Operations)
	return descriptor
}

func validateDescriptor(descriptor Descriptor) error {
	if descriptor.Name == "" {
		return errors.New("config resource adapter name is empty")
	}
	if len(descriptor.Operations) == 0 {
		return fmt.Errorf("config resource adapter %q declares no operations", descriptor.Name)
	}
	for _, operation := range descriptor.Operations {
		switch operation {
		case ReadList, ReadGet, ApplyCreate, ApplyUpdate, ApplyDelete:
		default:
			return fmt.Errorf("config resource adapter %q declares unknown operation %q", descriptor.Name, operation)
		}
	}
	if (containsString(descriptor.Operations, ApplyUpdate) || containsString(descriptor.Operations, ApplyDelete)) && descriptor.RevisionField == "" {
		return fmt.Errorf("config resource adapter %q must declare revision_field for update or delete", descriptor.Name)
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func compactStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
