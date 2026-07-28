// Package configresources provides the single routing seam used by the
// Config Manager agent. Product resources implement Adapter; the model only
// sees the stable config_read/config_apply protocol built on top of Registry.
package configresources

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
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

// ReadRequest describes a metadata, catalog, or exact-item read. Cursor and
// ResultMaxBytes are transport concerns: adapters only own resource access.
type ReadRequest struct {
	Operation      string
	Resource       string
	IDs            []string
	Scope          string
	Query          string
	Cursor         string
	Limit          int
	ResultMaxBytes int
}

// Catalog is the adapter-neutral input for a paginated list response.
// Metadata carries small resource-wide context such as editable Skill scopes.
type Catalog struct {
	Items    []any
	Metadata any
}

// NewCatalog converts a typed resource catalog without forcing adapters to
// encode and decode their values through JSON.
func NewCatalog[T any](items []T) Catalog {
	result := make([]any, len(items))
	for index := range items {
		result[index] = items[index]
	}
	return Catalog{Items: result}
}

type ReadFailure struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type ListResult struct {
	Schema     string `json:"schema"`
	Status     string `json:"status"`
	Resource   string `json:"resource"`
	Scope      string `json:"scope,omitempty"`
	Metadata   any    `json:"metadata,omitempty"`
	Items      []any  `json:"items"`
	Returned   int    `json:"returned"`
	Total      int    `json:"total"`
	Truncated  bool   `json:"truncated"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type GetResult struct {
	Schema     string        `json:"schema"`
	Status     string        `json:"status"`
	Resource   string        `json:"resource"`
	Scope      string        `json:"scope,omitempty"`
	Items      []any         `json:"items"`
	MissingIDs []string      `json:"missing_ids,omitempty"`
	Failures   []ReadFailure `json:"failures,omitempty"`
	Processed  int           `json:"processed"`
	Total      int           `json:"total"`
	Truncated  bool          `json:"truncated"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type missingResourceError struct{ err error }

func (err missingResourceError) Error() string { return err.err.Error() }
func (err missingResourceError) Unwrap() error { return err.err }

// Missing marks an adapter error as an exact-ID absence. Storage and decoding
// failures remain ordinary failures and are never mislabeled as missing.
func Missing(err error) error {
	if err == nil {
		return nil
	}
	return missingResourceError{err: err}
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
	request.Cursor = strings.TrimSpace(request.Cursor)
	request.IDs = compactStrings(request.IDs)
	if request.Limit < 0 {
		return nil, errors.New("config_read limit cannot be negative")
	}
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
		catalog, err := adapter.List(ctx, request)
		if err != nil {
			return nil, err
		}
		value, ok := catalog.(Catalog)
		if !ok {
			return nil, fmt.Errorf("config resource %q returned %T from list, want configresources.Catalog", request.Resource, catalog)
		}
		return paginateCatalog(value, request)
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
		return r.readExactItems(ctx, adapter, request)
	default:
		return nil, fmt.Errorf("unsupported config read operation %q", request.Operation)
	}
}

type readCursor struct {
	Version     int    `json:"v"`
	Operation   string `json:"op"`
	Offset      int    `json:"o"`
	Successes   int    `json:"s,omitempty"`
	Fingerprint string `json:"f"`
	CatalogHash string `json:"h,omitempty"`
}

func paginateCatalog(catalog Catalog, request ReadRequest) (ListResult, error) {
	catalogHash, err := hashCatalog(catalog)
	if err != nil {
		return ListResult{}, fmt.Errorf("hash config catalog: %w", err)
	}
	cursor, err := decodeReadCursor(request.Cursor, request, ReadList)
	if err != nil {
		return ListResult{}, err
	}
	if cursor.CatalogHash != "" && cursor.CatalogHash != catalogHash {
		return ListResult{}, errors.New("config_read cursor is stale because the catalog changed; restart list from the first page / 配置目录已变化，请从第一页重新读取")
	}
	if cursor.Offset > len(catalog.Items) {
		return ListResult{}, errors.New("config_read cursor is outside the current catalog / config_read 游标超出当前目录范围")
	}

	result := ListResult{
		Schema: "config.catalog.v1", Status: "success", Resource: request.Resource,
		Scope: request.Scope, Metadata: catalog.Metadata, Items: []any{}, Total: len(catalog.Items),
	}
	for index := cursor.Offset; index < len(catalog.Items); index++ {
		if request.Limit > 0 && len(result.Items) >= request.Limit {
			break
		}
		candidate := result
		candidate.Items = append(append([]any(nil), result.Items...), catalog.Items[index])
		candidate.Returned = len(candidate.Items)
		candidate.Truncated = index+1 < len(catalog.Items)
		if candidate.Truncated {
			candidate.Status = "partial"
			candidate.NextCursor, err = encodeReadCursor(readCursor{
				Version: 1, Operation: ReadList, Offset: index + 1,
				Fingerprint: readRequestFingerprint(request, ReadList), CatalogHash: catalogHash,
			})
			if err != nil {
				return ListResult{}, err
			}
		}
		if exceedsReadBudget(candidate, request.ResultMaxBytes) {
			if len(result.Items) == 0 {
				return ListResult{}, fmt.Errorf("config resource %q contains a catalog item larger than the %d-byte shared result budget / 单项配置超过共享结果预算", request.Resource, request.ResultMaxBytes)
			}
			break
		}
		result = candidate
	}
	result.Returned = len(result.Items)
	nextOffset := cursor.Offset + result.Returned
	result.Truncated = nextOffset < len(catalog.Items)
	if result.Truncated {
		result.Status = "partial"
		result.NextCursor, err = encodeReadCursor(readCursor{
			Version: 1, Operation: ReadList, Offset: nextOffset,
			Fingerprint: readRequestFingerprint(request, ReadList), CatalogHash: catalogHash,
		})
		if err != nil {
			return ListResult{}, err
		}
	} else {
		result.Status = "success"
		result.NextCursor = ""
	}
	return result, nil
}

func (r *Registry) readExactItems(ctx context.Context, adapter Adapter, request ReadRequest) (GetResult, error) {
	cursor, err := decodeReadCursor(request.Cursor, request, ReadGet)
	if err != nil {
		return GetResult{}, err
	}
	if cursor.Offset > len(request.IDs) {
		return GetResult{}, errors.New("config_read cursor is outside the requested ids / config_read 游标超出请求 ID 范围")
	}
	result := GetResult{
		Schema: "config.read.v1", Status: "success", Resource: request.Resource,
		Scope: request.Scope, Items: []any{}, Total: len(request.IDs),
	}
	successes := cursor.Successes
	for index := cursor.Offset; index < len(request.IDs); index++ {
		if request.Limit > 0 && result.Processed >= request.Limit {
			break
		}
		id := request.IDs[index]
		exact := request
		exact.IDs = []string{id}
		exact.Cursor = ""
		exact.Limit = 0
		exact.ResultMaxBytes = 0
		item, itemErr := adapter.Get(ctx, exact)
		candidate := result
		candidate.Items = append([]any(nil), result.Items...)
		candidate.MissingIDs = append([]string(nil), result.MissingIDs...)
		candidate.Failures = append([]ReadFailure(nil), result.Failures...)
		candidate.Processed++
		candidateSuccesses := successes
		switch {
		case itemErr == nil:
			candidate.Items = append(candidate.Items, item)
			candidateSuccesses++
		case isMissing(itemErr):
			candidate.MissingIDs = append(candidate.MissingIDs, id)
		default:
			candidate.Failures = append(candidate.Failures, ReadFailure{ID: id, Message: boundedError(itemErr)})
		}
		candidate.Truncated = index+1 < len(request.IDs)
		if candidate.Truncated {
			candidate.Status = "partial"
			candidate.NextCursor, err = encodeReadCursor(readCursor{
				Version: 1, Operation: ReadGet, Offset: index + 1, Successes: candidateSuccesses,
				Fingerprint: readRequestFingerprint(request, ReadGet),
			})
			if err != nil {
				return GetResult{}, err
			}
		}
		if exceedsReadBudget(candidate, request.ResultMaxBytes) {
			if result.Processed == 0 {
				return GetResult{}, fmt.Errorf("config resource %q item %q is larger than the %d-byte shared result budget / 单项配置超过共享结果预算", request.Resource, id, request.ResultMaxBytes)
			}
			break
		}
		result = candidate
		successes = candidateSuccesses
	}
	nextOffset := cursor.Offset + result.Processed
	result.Truncated = nextOffset < len(request.IDs)
	if result.Truncated {
		result.Status = "partial"
		result.NextCursor, err = encodeReadCursor(readCursor{
			Version: 1, Operation: ReadGet, Offset: nextOffset, Successes: successes,
			Fingerprint: readRequestFingerprint(request, ReadGet),
		})
		if err != nil {
			return GetResult{}, err
		}
	} else {
		result.NextCursor = ""
		if len(result.MissingIDs) > 0 || len(result.Failures) > 0 {
			result.Status = "partial"
		}
	}
	if !result.Truncated && successes == 0 {
		return GetResult{}, fmt.Errorf("config_read found none of the %d requested %s ids / 请求的 ID 全部未找到或读取失败", len(request.IDs), request.Resource)
	}
	return result, nil
}

func isMissing(err error) bool {
	var marked missingResourceError
	return errors.As(err, &marked) || errors.Is(err, fs.ErrNotExist)
}

func boundedError(err error) string {
	const maxBytes = 4096
	message := strings.TrimSpace(err.Error())
	if len(message) <= maxBytes {
		return message
	}
	message = message[:maxBytes]
	for len(message) > 0 && !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return message + "…"
}

func exceedsReadBudget(value any, maxBytes int) bool {
	if maxBytes <= 0 {
		return false
	}
	encoded, err := json.Marshal(value)
	return err != nil || len(encoded) > maxBytes
}

func hashCatalog(catalog Catalog) (string, error) {
	encoded, err := json.Marshal(catalog)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16]), nil
}

func encodeReadCursor(cursor readCursor) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeReadCursor(value string, request ReadRequest, operation string) (readCursor, error) {
	if strings.TrimSpace(value) == "" {
		return readCursor{Version: 1, Operation: operation, Fingerprint: readRequestFingerprint(request, operation)}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return readCursor{}, errors.New("config_read cursor is invalid / config_read 游标无效")
	}
	var cursor readCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || cursor.Operation != operation || cursor.Offset < 0 || cursor.Successes < 0 {
		return readCursor{}, errors.New("config_read cursor is invalid / config_read 游标无效")
	}
	if cursor.Fingerprint != readRequestFingerprint(request, operation) {
		return readCursor{}, errors.New("config_read cursor does not belong to this request / config_read 游标不属于当前请求")
	}
	return cursor, nil
}

func readRequestFingerprint(request ReadRequest, operation string) string {
	payload := struct {
		Operation string   `json:"operation"`
		Resource  string   `json:"resource"`
		IDs       []string `json:"ids,omitempty"`
		Scope     string   `json:"scope,omitempty"`
		Query     string   `json:"query,omitempty"`
	}{operation, request.Resource, request.IDs, request.Scope, request.Query}
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:16])
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
