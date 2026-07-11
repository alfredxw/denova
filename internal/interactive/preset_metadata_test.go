package interactive

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPresetResourcesDropLegacyTopLevelTags(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "teller", value: &Teller{}},
		{name: "story director", value: &StoryDirector{}},
		{name: "event package", value: &EventPackageModule{}},
		{name: "rule system", value: &RuleSystemModule{}},
		{name: "actor state", value: &ActorStateModule{}},
		{name: "memory structure", value: &StoryMemoryStructureModule{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(`{"id":"legacy","tags":["unused"]}`), test.value); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(`"tags"`)) {
				t.Fatalf("legacy preset tags leaked into persisted JSON: %s", encoded)
			}
		})
	}
}
