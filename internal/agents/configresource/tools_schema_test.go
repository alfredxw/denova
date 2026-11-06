package configresource

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"denova/config"
)

func TestConfigurationToolResourceSchemaMatchesAvailableRegistry(t *testing.T) {
	for _, workspace := range []bool{false, true} {
		name := "without_workspace"
		cfg := &config.Config{NovaDir: t.TempDir()}
		if workspace {
			name = "with_workspace"
			cfg.Workspace = t.TempDir()
		}
		t.Run(name, func(t *testing.T) {
			definitions, err := NewTools(cfg, 0)
			if err != nil {
				t.Fatal(err)
			}
			read := configManagerToolByName(t, cfg, "config_read")
			output, err := runToolForTest(context.Background(), read, `{"operation":"describe"}`)
			if err != nil {
				t.Fatal(err)
			}
			var descriptors []Descriptor
			if err := json.Unmarshal([]byte(output), &descriptors); err != nil {
				t.Fatal(err)
			}
			var names []any
			for _, descriptor := range descriptors {
				names = append(names, descriptor.Name)
			}
			for _, definition := range definitions {
				info, err := definition.Tool.Info(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				schema, err := info.ToJSONSchema()
				if err != nil {
					t.Fatal(err)
				}
				resource, ok := schema.Properties.Get("resource")
				if !ok || !reflect.DeepEqual(resource.Enum, names) {
					t.Errorf("%s resource schema = %#v, want registered resource enum %v", info.Name, resource, names)
				}
			}
		})
	}
}
