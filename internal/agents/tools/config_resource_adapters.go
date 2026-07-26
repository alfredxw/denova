package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"denova/internal/configresources"
)

type configResourceAdapter struct {
	descriptor configresources.Descriptor
	list       func(context.Context, configresources.ReadRequest) (any, error)
	get        func(context.Context, configresources.ReadRequest) (any, error)
	apply      func(context.Context, configresources.Mutation) (any, error)
}

func (a configResourceAdapter) Descriptor() configresources.Descriptor { return a.descriptor }

func (a configResourceAdapter) List(ctx context.Context, request configresources.ReadRequest) (any, error) {
	if a.list == nil {
		return nil, fmt.Errorf("config resource %s does not support list", a.descriptor.Name)
	}
	return a.list(ctx, request)
}

func (a configResourceAdapter) Get(ctx context.Context, request configresources.ReadRequest) (any, error) {
	if a.get == nil {
		return nil, fmt.Errorf("config resource %s does not support get", a.descriptor.Name)
	}
	return a.get(ctx, request)
}

func (a configResourceAdapter) Apply(ctx context.Context, mutation configresources.Mutation) (any, error) {
	if a.apply == nil {
		return nil, fmt.Errorf("config resource %s is read-only", a.descriptor.Name)
	}
	return a.apply(ctx, mutation)
}

func decodeConfigValue(value map[string]any, destination any) error {
	if len(value) == 0 {
		return errors.New("config resource value is required")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode config resource value: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("invalid config resource value: %w", err)
	}
	return nil
}

func requireConfigRevision(resource, id, expected, actual string) error {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" {
		return fmt.Errorf("%s %q requires a revision from config_read", resource, id)
	}
	if expected != actual {
		return fmt.Errorf("%s %q revision conflict: expected %s, current %s", resource, id, expected, actual)
	}
	return nil
}

func normalizeConfigIDs(ids []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
	}
	return result
}

type configMutationReceipt struct {
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
	ID        string `json:"id"`
	Revision  string `json:"revision,omitempty"`
	Value     any    `json:"value,omitempty"`
}
