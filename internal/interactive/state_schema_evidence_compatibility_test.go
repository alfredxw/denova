package interactive

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHistoricalRequirementEvidenceKindIsIgnored(t *testing.T) {
	base := StoryDirectorActorStateSystem{Templates: []ActorStateTemplate{{
		ID: "protagonist", Fields: []ActorStateField{{Name: "身份", Type: "string"}},
	}}}
	var proposal ActorStateSchemaProposal
	if err := json.Unmarshal([]byte(`{
		"requirements":[{
			"source":{"kind":"opening","id":"opening-draft"},
			"requirement":"主角身份需要长期记录",
			"evidence_kind":"legacy-value",
			"value_policy":"schema_only",
			"expected_type":"string",
			"decision":"covered",
			"template_id":"protagonist",
			"field_id":"身份"
		}],
		"adaptation":{}
	}`), &proposal); err != nil {
		t.Fatalf("decode historical schema proposal: %v", err)
	}

	normalized, _, err := ValidateOpeningGameStateSchemaProposal(base, StoryDirectorTRPGSystem{}, proposal)
	if err != nil {
		t.Fatalf("historical evidence_kind must not affect schema validation: %v", err)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		t.Fatalf("encode normalized schema proposal: %v", err)
	}
	if strings.Contains(string(encoded), `"evidence_kind"`) {
		t.Fatalf("historical evidence_kind must not survive validation: %s", encoded)
	}
}

func TestHistoricalActorValueSourceEvidenceKindIsIgnored(t *testing.T) {
	adaptation, err := ParseActorStateSchemaAdaptation(`{
		"actor_ops":[{
			"op":"set",
			"actor_id":"protagonist",
			"field_id":"身份",
			"value":"继承人",
			"value_source":{
				"source_id":"state_schema_batch:identity",
				"item_id":"identity",
				"source":{"kind":"opening","id":"opening-draft"},
				"evidence_kind":"confirmed"
			}
		}]
	}`)
	if err != nil {
		t.Fatalf("parse historical actor value source: %v", err)
	}
	encoded, err := json.Marshal(adaptation)
	if err != nil {
		t.Fatalf("encode normalized actor value source: %v", err)
	}
	if strings.Contains(string(encoded), `"evidence_kind"`) {
		t.Fatalf("historical value-source evidence_kind must not survive parsing: %s", encoded)
	}
	if !strings.Contains(string(encoded), `"source":{"kind":"opening","id":"opening-draft"}`) {
		t.Fatalf("removing evidence_kind must preserve the bounded source: %s", encoded)
	}
}
