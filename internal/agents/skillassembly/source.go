package skillassembly

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	publictools "github.com/alfredxw/denova/agent/tools"

	novaskills "denova/internal/agents/skills"
)

const skillSourceID = "denova.skills"

type skillSource struct {
	backend  *novaskills.Backend
	maxBytes int
	identity agent.CapabilityIdentity
}

func newSkillSource(backend *novaskills.Backend, maxBytes int, identityConfig any) *skillSource {
	encoded, _ := json.Marshal(identityConfig)
	digest := sha256.Sum256(encoded)
	return &skillSource{
		backend: backend, maxBytes: maxBytes,
		identity: agent.CapabilityIdentity{Kind: "denova.skills.source", Version: 1, ConfigHash: hex.EncodeToString(digest[:])},
	}
}

func (source *skillSource) Identity() agent.CapabilityIdentity { return source.identity }

func (source *skillSource) List(ctx context.Context, query publictools.SkillQuery) ([]publictools.Skill, error) {
	available, err := source.backend.List(ctx)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	limit := query.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	result := make([]publictools.Skill, 0, min(limit, len(available)))
	for _, item := range available {
		if needle != "" && !strings.Contains(strings.ToLower(item.Name+"\n"+item.Description), needle) {
			continue
		}
		result = append(result, publictools.Skill{
			Ref:  publictools.SkillRef{Source: skillSourceID, ID: item.Name},
			Name: item.Name, Description: item.Description,
		})
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func (source *skillSource) Read(ctx context.Context, ref publictools.SkillRef) (publictools.SkillContent, error) {
	if strings.TrimSpace(ref.Source) != skillSourceID {
		return publictools.SkillContent{}, fmt.Errorf("unsupported Skill source %q", ref.Source)
	}
	name := strings.TrimSpace(ref.ID)
	skill, err := source.backend.Get(ctx, name)
	if err != nil {
		return publictools.SkillContent{}, err
	}
	instructions := novaskills.FormatForModel(skill, source.maxBytes)
	digest := sha256.Sum256([]byte(instructions))
	return publictools.SkillContent{
		Skill:    publictools.Skill{Ref: publictools.SkillRef{Source: skillSourceID, ID: skill.Name}, Name: skill.Name, Description: skill.Description},
		Revision: hex.EncodeToString(digest[:]), Instructions: instructions,
	}, nil
}
