package toolapproval

import (
	"encoding/json"
	"strings"

	"denova/config"
	"mvdan.cc/sh/v3/syntax"
)

// workspaceRuleProposal derives a reusable authorization only when the current
// command can be parsed into one static call and every future match will pass
// the same workspace-aware validator again. Dynamic shell syntax, environment
// overrides, compound commands, and unknown executables remain one-shot only.
func workspaceRuleProposal(toolName, command, workspace, cwd string) *RuleProposal {
	boundary, ok := newPathBoundary(workspace, cwd)
	if !ok {
		return nil
	}
	var words []string
	if strings.EqualFold(strings.TrimSpace(toolName), "pwsh") {
		if strings.ContainsAny(command, "$`{}()><") || strings.Contains(command, "&&") || strings.Contains(command, "||") {
			return nil
		}
		segments := strings.Split(command, "|")
		if len(segments) != 1 {
			return nil
		}
		words, ok = splitSimplePowerShellWords(segments[0])
		if !ok || len(words) == 0 {
			return nil
		}
		class := classifyPowerShellCall(words, boundary, config.AgentApprovalWrite)
		if class != commandWrite && class != commandNetworkRead && !rememberableGitPush(words) {
			return nil
		}
	} else {
		file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
		if err != nil {
			return nil
		}
		calls, literal := literalBashCalls(file)
		if !literal || len(calls) != 1 {
			return nil
		}
		words = calls[0]
		class := classifyLiteralCommand(words, boundary, config.AgentApprovalWrite)
		if class != commandWrite && class != commandNetworkRead && !rememberableGitPush(words) {
			return nil
		}
	}

	identity, pattern := reusableCommandIdentity(words)
	if len(identity) == 0 || pattern == "" {
		return nil
	}
	encoded, err := json.Marshal(identity)
	if err != nil {
		return nil
	}
	return &RuleProposal{
		ToolName:       strings.ToLower(strings.TrimSpace(toolName)),
		Matcher:        config.AgentApprovalMatcherShell,
		MatcherVersion: config.AgentApprovalRuleMatcherVersion,
		MatchKey:       string(encoded),
		DisplayPattern: pattern,
	}
}

func reusableCommandIdentity(words []string) ([]string, string) {
	if len(words) == 0 {
		return nil, ""
	}
	name := strings.ToLower(strings.TrimSpace(words[0]))
	args := words[1:]

	switch name {
	case "go":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, ""
		}
		identity := []string{name, strings.ToLower(args[0])}
		if strings.EqualFold(args[0], "mod") && len(args) > 1 {
			if strings.HasPrefix(args[1], "-") {
				return nil, ""
			}
			identity = append(identity, strings.ToLower(args[1]))
		}
		return identity, strings.Join(identity, " ") + " ..."
	case "cargo":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, ""
		}
		identity := []string{name, strings.ToLower(args[0])}
		return identity, strings.Join(identity, " ") + " ..."
	case "git":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, ""
		}
		verb := strings.ToLower(args[0])
		switch verb {
		case "add", "commit":
			identity := []string{name, verb}
			return identity, strings.Join(identity, " ") + " ..."
		case "push":
			if !rememberableGitPush(words) {
				return nil, ""
			}
			remote := gitPushRemote(args[1:])
			identity := []string{name, verb, remote}
			if remote == "<default>" {
				return identity, "git push ..."
			}
			return identity, "git push " + remote + " ..."
		default:
			return nil, ""
		}
	case "npx", "bunx":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, ""
		}
		identity := []string{name, args[0]}
		return identity, strings.Join(identity, " ") + " ..."
	case "npm", "pnpm", "yarn", "bun":
		if len(args) == 0 || strings.HasPrefix(args[0], "-") {
			return nil, ""
		}
		verb := strings.ToLower(args[0])
		identity := []string{name, verb}
		if oneOf(verb, "run", "run-script", "exec", "dlx", "x") {
			if len(args) < 2 || strings.HasPrefix(args[1], "-") {
				return nil, ""
			}
			identity = append(identity, args[1])
		}
		return identity, strings.Join(identity, " ") + " ..."
	case "mkdir", "touch", "cp", "mv", "rm", "rmdir", "tee",
		"new-item", "set-content", "add-content", "copy-item", "move-item", "remove-item":
		return []string{name}, name + " ..."
	default:
		// Ambiguous command lines (network clients, build frontends, and
		// commands whose entry point can be replaced by flags) stay one-shot.
		// A new family must define its stable semantic boundary explicitly here.
		return nil, ""
	}
}

// rememberableGitPush intentionally accepts only the common non-destructive
// form. Force, delete, mirror, helper override, and other option-bearing pushes
// remain one-shot approvals because their impact is materially broader.
func rememberableGitPush(words []string) bool {
	if len(words) < 2 || !strings.EqualFold(words[0], "git") || !strings.EqualFold(words[1], "push") {
		return false
	}
	positional := make([]string, 0, len(words)-2)
	for _, arg := range words[2:] {
		if strings.HasPrefix(arg, "-") && arg != "-u" && !strings.EqualFold(arg, "--set-upstream") {
			return false
		}
		if !strings.HasPrefix(arg, "-") {
			positional = append(positional, arg)
		}
	}
	if len(positional) > 0 && strings.HasPrefix(strings.ToLower(positional[0]), "ext::") {
		return false
	}
	for _, refspec := range positional[1:] {
		if strings.HasPrefix(refspec, "+") || strings.HasPrefix(refspec, ":") {
			return false
		}
	}
	return true
}

func gitPushRemote(args []string) string {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return "<default>"
}
