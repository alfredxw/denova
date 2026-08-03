package toolapproval

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// bashStaticCall is the policy-visible projection of one simple command. The
// boundary is captured per call because a validated cd changes how subsequent
// relative paths must be checked.
type bashStaticCall struct {
	words    []string
	boundary pathBoundary
}

type bashStaticFlow struct {
	guaranteedSuccess bool
	changesState      bool
}

type bashStaticAnalyzer struct {
	boundary  pathBoundary
	variables map[string]string
	calls     []bashStaticCall
}

// staticBashCalls accepts shell composition only while every command word can
// be resolved without executing shell code. It deliberately understands a
// small amount of shell state (literal local assignments and validated cd)
// so ordinary multi-step workspace inspection does not become "dynamic"
// merely because the model named a file once and reused it.
func staticBashCalls(file *syntax.File, boundary pathBoundary) ([]bashStaticCall, bool) {
	if file == nil {
		return nil, false
	}
	analyzer := bashStaticAnalyzer{
		boundary:  boundary,
		variables: make(map[string]string),
		calls:     make([]bashStaticCall, 0),
	}
	for _, statement := range file.Stmts {
		if _, ok := analyzer.analyzeStatement(statement, false); !ok {
			return nil, false
		}
	}
	return analyzer.calls, true
}

func (analyzer *bashStaticAnalyzer) analyzeStatement(statement *syntax.Stmt, inPipeline bool) (bashStaticFlow, bool) {
	if statement == nil || statement.Background || statement.Coprocess || statement.Disown ||
		statement.Negated || len(statement.Redirs) != 0 {
		return bashStaticFlow{}, false
	}
	switch command := statement.Cmd.(type) {
	case *syntax.CallExpr:
		return analyzer.analyzeCall(command, inPipeline)
	case *syntax.BinaryCmd:
		switch command.Op {
		case syntax.AndStmt:
			left, ok := analyzer.analyzeStatement(command.X, inPipeline)
			if !ok {
				return bashStaticFlow{}, false
			}
			right, ok := analyzer.analyzeStatement(command.Y, inPipeline)
			if !ok || !left.guaranteedSuccess && right.changesState {
				// A state mutation behind a command that may fail would make later
				// expansion depend on inherited shell state rather than the text
				// inspected here.
				return bashStaticFlow{}, false
			}
			return bashStaticFlow{
				guaranteedSuccess: left.guaranteedSuccess && right.guaranteedSuccess,
				changesState:      left.changesState || right.changesState,
			}, true
		case syntax.OrStmt:
			return analyzer.analyzeAlternative(command)
		case syntax.Pipe, syntax.PipeAll:
			return analyzer.analyzePipeline(command)
		default:
			return bashStaticFlow{}, false
		}
	default:
		// Loops, functions, subshells, declarations, tests, and arithmetic
		// commands remain approval-gated until they have their own validator.
		return bashStaticFlow{}, false
	}
}

func (analyzer *bashStaticAnalyzer) analyzeAlternative(command *syntax.BinaryCmd) (bashStaticFlow, bool) {
	left := analyzer.cloneWithoutCalls()
	leftFlow, leftOK := left.analyzeStatement(command.X, false)
	right := analyzer.cloneWithoutCalls()
	rightFlow, rightOK := right.analyzeStatement(command.Y, false)
	if !leftOK || !rightOK || leftFlow.changesState || rightFlow.changesState {
		return bashStaticFlow{}, false
	}
	analyzer.calls = append(analyzer.calls, left.calls...)
	analyzer.calls = append(analyzer.calls, right.calls...)
	return bashStaticFlow{guaranteedSuccess: leftFlow.guaranteedSuccess || rightFlow.guaranteedSuccess}, true
}

func (analyzer *bashStaticAnalyzer) analyzePipeline(command *syntax.BinaryCmd) (bashStaticFlow, bool) {
	left := analyzer.cloneWithoutCalls()
	leftFlow, leftOK := left.analyzeStatement(command.X, true)
	right := analyzer.cloneWithoutCalls()
	rightFlow, rightOK := right.analyzeStatement(command.Y, true)
	if !leftOK || !rightOK || leftFlow.changesState || rightFlow.changesState {
		// Pipeline elements run in shell-specific subshell contexts. Refuse to
		// infer persistent variables or cwd from either side.
		return bashStaticFlow{}, false
	}
	analyzer.calls = append(analyzer.calls, left.calls...)
	analyzer.calls = append(analyzer.calls, right.calls...)
	return bashStaticFlow{}, true
}

func (analyzer *bashStaticAnalyzer) analyzeCall(call *syntax.CallExpr, inPipeline bool) (bashStaticFlow, bool) {
	if call == nil {
		return bashStaticFlow{}, false
	}
	if len(call.Args) == 0 {
		if inPipeline || len(call.Assigns) == 0 || !analyzer.applyAssignments(call.Assigns) {
			return bashStaticFlow{}, false
		}
		return bashStaticFlow{guaranteedSuccess: true, changesState: true}, true
	}
	// Prefix assignments alter command resolution and runtime behavior. They
	// remain interactive even when their values happen to be literal.
	if len(call.Assigns) != 0 {
		return bashStaticFlow{}, false
	}
	words := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		value, ok := staticBashWord(word, analyzer.variables, false)
		if !ok {
			return bashStaticFlow{}, false
		}
		words = append(words, value)
	}
	if len(words) == 0 || strings.TrimSpace(words[0]) == "" {
		return bashStaticFlow{}, false
	}
	callBoundary := analyzer.boundary
	analyzer.calls = append(analyzer.calls, bashStaticCall{words: words, boundary: callBoundary})
	name := strings.ToLower(words[0])
	if name == "cd" {
		if inPipeline {
			return bashStaticFlow{}, false
		}
		next, ok := analyzer.boundary.changeDirectory(words[1:])
		if !ok {
			return bashStaticFlow{}, false
		}
		analyzer.boundary = next
		return bashStaticFlow{guaranteedSuccess: true, changesState: true}, true
	}
	return bashStaticFlow{guaranteedSuccess: shellCallNormallySucceeds(name)}, true
}

func (analyzer *bashStaticAnalyzer) applyAssignments(assignments []*syntax.Assign) bool {
	for _, assignment := range assignments {
		if assignment == nil || assignment.Append || assignment.Naked || assignment.Name == nil ||
			assignment.Index != nil || assignment.Array != nil || !safeLocalShellVariableName(assignment.Name.Value) {
			return false
		}
		value := ""
		if assignment.Value != nil {
			resolved, ok := staticBashWord(assignment.Value, analyzer.variables, true)
			if !ok {
				return false
			}
			value = resolved
		}
		analyzer.variables[assignment.Name.Value] = value
	}
	return true
}

func (analyzer bashStaticAnalyzer) cloneWithoutCalls() bashStaticAnalyzer {
	variables := make(map[string]string, len(analyzer.variables))
	for name, value := range analyzer.variables {
		variables[name] = value
	}
	return bashStaticAnalyzer{
		boundary:  analyzer.boundary,
		variables: variables,
		calls:     make([]bashStaticCall, 0),
	}
}

func staticBashWord(word *syntax.Word, variables map[string]string, assignment bool) (string, bool) {
	if word == nil {
		return "", false
	}
	var result strings.Builder
	var appendParts func([]syntax.WordPart, bool) bool
	appendParts = func(parts []syntax.WordPart, quoted bool) bool {
		for _, part := range parts {
			switch value := part.(type) {
			case *syntax.Lit:
				if !quoted && !assignment && strings.ContainsAny(value.Value, "*?[]{}") {
					return false
				}
				result.WriteString(value.Value)
			case *syntax.SglQuoted:
				if value.Dollar {
					return false
				}
				result.WriteString(value.Value)
			case *syntax.DblQuoted:
				if value.Dollar || !appendParts(value.Parts, true) {
					return false
				}
			case *syntax.ParamExp:
				if !quoted && !assignment || !simpleBashParameter(value) {
					return false
				}
				resolved, ok := variables[value.Param.Value]
				if !ok {
					return false
				}
				result.WriteString(resolved)
			default:
				return false
			}
		}
		return true
	}
	ok := appendParts(word.Parts, assignment)
	return result.String(), ok
}

func simpleBashParameter(parameter *syntax.ParamExp) bool {
	return parameter != nil && parameter.Param != nil && parameter.Flags == nil && parameter.NestedParam == nil &&
		parameter.Index == nil && len(parameter.Modifiers) == 0 && parameter.Slice == nil && parameter.Repl == nil &&
		parameter.Names == 0 && parameter.Exp == nil && !parameter.Excl && !parameter.Length &&
		!parameter.Width && !parameter.IsSet
}

func safeLocalShellVariableName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for index := 1; index < len(name); index++ {
		character := name[index]
		if character != '_' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func shellCallNormallySucceeds(name string) bool {
	switch name {
	case ":", "true", "echo", "printf", "pwd":
		return true
	default:
		return false
	}
}
