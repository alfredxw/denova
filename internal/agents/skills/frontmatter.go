package skills

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type frontMatterFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Agent       string `yaml:"agent,omitempty"`
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

func marshalFrontmatter(name, description string, agents []string) string {
	data, err := yaml.Marshal(frontMatterFile{Name: name, Description: description, Agent: strings.Join(agents, ",")})
	if err != nil {
		agentLine := ""
		if len(agents) > 0 {
			agentLine = fmt.Sprintf("agent: %q\n", strings.Join(agents, ","))
		}
		return fmt.Sprintf("name: %q\ndescription: %q\n%s", name, description, agentLine)
	}
	return string(data)
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
