package skills

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type frontMatterFile struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Category     string   `yaml:"category,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty"`
	Agent        string   `yaml:"agent,omitempty"`
}

func ValidateName(name string) error {
	if !skillNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("skill name must match %s", skillNamePattern.String())
	}
	return nil
}

func parseFrontmatter(data string) (string, string, error) {
	const delimiter = "---"
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, delimiter) {
		return "", "", fmt.Errorf("file does not start with frontmatter delimiter")
	}
	rest := data[len(delimiter):]
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return "", "", fmt.Errorf("frontmatter closing delimiter not found")
	}
	frontmatter := strings.TrimSpace(rest[:endIdx])
	content := rest[endIdx+len("\n"+delimiter):]
	if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}
	return frontmatter, content, nil
}

func marshalFrontmatter(name string, metadata CreateMetadata) string {
	metadata = normalizeCreateMetadata(metadata)
	data, err := yaml.Marshal(frontMatterFile{
		Name:         name,
		Description:  metadata.Description,
		Category:     metadata.Category,
		Capabilities: metadata.Capabilities,
		Agent:        strings.Join(metadata.Agents, ","),
	})
	if err != nil {
		agentLine := ""
		if len(metadata.Agents) > 0 {
			agentLine = fmt.Sprintf("agent: %q\n", strings.Join(metadata.Agents, ","))
		}
		capabilityLine := ""
		if len(metadata.Capabilities) > 0 {
			quoted := make([]string, 0, len(metadata.Capabilities))
			for _, capability := range metadata.Capabilities {
				quoted = append(quoted, fmt.Sprintf("%q", capability))
			}
			capabilityLine = fmt.Sprintf("capabilities: [%s]\n", strings.Join(quoted, ", "))
		}
		return fmt.Sprintf("name: %q\ndescription: %q\ncategory: %q\n%s%s", name, metadata.Description, metadata.Category, capabilityLine, agentLine)
	}
	return string(data)
}

func normalizeCreateMetadata(metadata CreateMetadata) CreateMetadata {
	metadata.Description = strings.TrimSpace(metadata.Description)
	metadata.Category = normalizeCategory(metadata.Category)
	metadata.Capabilities = normalizeCapabilities(metadata.Capabilities)
	metadata.Agents = normalizeAgentList(metadata.Agents)
	return metadata
}

func normalizeCategory(category string) string {
	if category = strings.TrimSpace(category); category != "" {
		return category
	}
	return CategoryGeneral
}

func normalizeCapabilities(capabilities []string) []string {
	seen := make(map[string]bool, len(capabilities))
	out := make([]string, 0, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.TrimSpace(capability)
		if capability == "" || seen[capability] {
			continue
		}
		seen[capability] = true
		out = append(out, capability)
	}
	return out
}

func normalizeAgentList(agents []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(agents))
	for _, agent := range agents {
		agent = strings.TrimSpace(agent)
		if agent == "" || seen[agent] {
			continue
		}
		seen[agent] = true
		out = append(out, agent)
	}
	return out
}
