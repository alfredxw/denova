package toolapproval

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
)

func evaluateFilesystemRead(request Request) (Decision, bool) {
	if request.Descriptor.Capability != config.AgentToolFilesystemRead {
		return Decision{}, false
	}
	plan, handled, err := agenttools.PlanFilesystemRead(request.Workspace, request.ToolName, request.Arguments)
	if !handled {
		return Decision{}, false
	}
	if err != nil {
		// Execution owns user-facing tool validation. The shared planner failed
		// before producing an external target, so allowing the call cannot widen
		// access and preserves the tool's precise model-visible diagnostic.
		return allow("filesystem_read_validation", RiskLow, "Filesystem read validation remains authoritative at the tool boundary."), true
	}
	if len(plan.External) == 0 {
		return allow("project_filesystem_read", RiskLow, "The filesystem read is contained by the current Project."), true
	}
	if attachmentReadsAllowed(request.AttachmentPaths, plan.External) {
		return allow("attached_file_read", RiskLow, "Every external target is a file explicitly attached by the user."), true
	}
	if request.Mode == config.AgentApprovalFullAccess {
		return allow("external_filesystem_read_full_access", RiskMedium, "Full Access allows this external filesystem read."), true
	}
	if filesystemReadAllowed(request, plan.External) {
		return allow("remembered_filesystem_read", RiskMedium, "A user-owned external filesystem read rule covers every requested target."), true
	}
	proposal, err := filesystemReadRuleProposal(plan.External)
	if err != nil {
		decision := prompt("external_filesystem_read", RiskMedium,
			"该工具将读取当前项目之外的本地文件，需要你的确认。 / This tool will read local files outside the current Project and requires approval.")
		decision.Details = filesystemReadDisplay(plan.External)
		return decision, true
	}
	decision := prompt("external_filesystem_read", RiskMedium,
		"该工具将读取当前项目之外的本地文件，需要你的确认。 / This tool will read local files outside the current Project and requires approval.")
	decision.Details = proposal.DisplayPattern
	decision.Remember = &proposal
	return decision, true
}

func attachmentReadsAllowed(paths []string, requested []agenttools.FilesystemReadGrant) bool {
	if len(paths) == 0 || len(requested) == 0 {
		return false
	}
	for _, target := range requested {
		covered := false
		for _, path := range paths {
			canonical, err := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
			if err != nil {
				continue
			}
			grant := agenttools.FilesystemReadGrant{Path: canonical}
			if agenttools.FilesystemReadGrantContains(grant, target) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func filesystemReadRuleProposal(grants []agenttools.FilesystemReadGrant) (RuleProposal, error) {
	encoded, err := json.Marshal(grants)
	if err != nil {
		return RuleProposal{}, fmt.Errorf("encode filesystem read roots: %w", err)
	}
	return RuleProposal{
		ToolName:       config.AgentApprovalFilesystemReadTool,
		Matcher:        config.AgentApprovalMatcherFilesystem,
		MatcherVersion: config.AgentApprovalRuleMatcherVersion,
		MatchKey:       string(encoded),
		DisplayPattern: filesystemReadDisplay(grants),
	}, nil
}

func filesystemReadDisplay(grants []agenttools.FilesystemReadGrant) string {
	values := make([]string, 0, len(grants))
	for _, grant := range grants {
		value := strings.TrimSpace(grant.Path)
		if grant.Recursive {
			value = strings.TrimRight(value, "/\\") + "/**"
		}
		values = append(values, value)
	}
	return strings.Join(values, "\n")
}

func filesystemReadAllowed(request Request, requested []agenttools.FilesystemReadGrant) bool {
	covered := make([]bool, len(requested))
	workspace := canonicalWorkspace(request.Workspace)
	projectID := strings.TrimSpace(request.ProjectID)
	for _, rule := range request.Rules {
		if rule.Scope != config.AgentApprovalRuleWorkspace || rule.ToolName != config.AgentApprovalFilesystemReadTool ||
			rule.Matcher != config.AgentApprovalMatcherFilesystem || rule.MatcherVersion != config.AgentApprovalRuleMatcherVersion {
			continue
		}
		if rule.ProjectID != "" && rule.ProjectID != projectID {
			continue
		}
		if ruleWorkspace := canonicalWorkspace(rule.Workspace); ruleWorkspace != "" && ruleWorkspace != workspace {
			continue
		}
		var granted []agenttools.FilesystemReadGrant
		if err := json.Unmarshal([]byte(rule.MatchKey), &granted); err != nil {
			continue
		}
		for index, target := range requested {
			if covered[index] {
				continue
			}
			for _, grant := range granted {
				if agenttools.FilesystemReadGrantContains(grant, target) {
					covered[index] = true
					break
				}
			}
		}
	}
	for _, allowed := range covered {
		if !allowed {
			return false
		}
	}
	return true
}
