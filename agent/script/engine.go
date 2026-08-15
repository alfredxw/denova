package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja/parser"
)

const (
	defaultMaxSourceBytes   = 1024 * 1024
	defaultMaxOutputBytes   = 128 * 1024
	defaultMaxCallStackSize = 1024
	wrappedSourceLineOffset = 1
)

// Engine is the single in-process JavaScript implementation used by both the
// immediate script tool and saved Script Tools.
type Engine struct {
	config Config
}

// NewEngine validates limits once so Compile and Run have no implicit policy.
func NewEngine(config Config) (*Engine, error) {
	if config.MaxSourceBytes == 0 {
		config.MaxSourceBytes = defaultMaxSourceBytes
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = defaultMaxOutputBytes
	}
	if config.MaxCallStackSize == 0 {
		config.MaxCallStackSize = defaultMaxCallStackSize
	}
	if config.MaxSourceBytes < 0 || config.MaxOutputBytes < 0 || config.MaxCallStackSize < 0 {
		return nil, errors.New("script engine limits must be positive")
	}
	return &Engine{config: config}, nil
}

// Compile wraps source as a strict function body. The wrapper is an internal
// implementation detail; diagnostics are translated back to user line numbers.
func (engine *Engine) Compile(ctx context.Context, source Source) (Program, []Diagnostic) {
	if engine == nil {
		return Program{}, []Diagnostic{{Kind: "script_engine_unavailable", Message: "Script engine is unavailable."}}
	}
	if err := contextError(ctx); err != nil {
		return Program{}, []Diagnostic{{Kind: "script_cancelled", Message: "Script compilation was cancelled."}}
	}
	if !utf8.ValidString(source.Code) {
		return Program{}, []Diagnostic{{Kind: "script_source_invalid", Message: "Script source must be valid UTF-8.", Path: source.Name}}
	}
	if len(source.Code) > engine.config.MaxSourceBytes {
		return Program{}, []Diagnostic{{
			Kind:    "script_source_limit",
			Message: fmt.Sprintf("Script source exceeds the configured %d byte limit.", engine.config.MaxSourceBytes),
			Path:    source.Name,
		}}
	}
	name := strings.TrimSpace(source.Name)
	if name == "" {
		name = "script.js"
	}
	wrapped := "(function(ctx, input) { \"use strict\";\n" + source.Code + "\n})"
	parsed, err := parser.ParseFile(nil, name, wrapped, 0)
	if err != nil {
		return Program{}, []Diagnostic{compileDiagnostic(name, err, strings.Count(source.Code, "\n")+1)}
	}
	compiled, err := goja.CompileAST(parsed, true)
	if err != nil {
		return Program{}, []Diagnostic{compileDiagnostic(name, err, strings.Count(source.Code, "\n")+1)}
	}
	return Program{compiled: compiled, name: name}, nil
}

// Run creates a fresh Runtime, installs the small synchronous Host API, and
// returns a JSON value. No JavaScript value survives this call.
func (engine *Engine) Run(
	ctx context.Context,
	program Program,
	host Host,
	input json.RawMessage,
) (result RunResult, err error) {
	defer func() {
		if value := recover(); value != nil {
			result = RunResult{}
			err = fmt.Errorf("script engine panic: %v", value)
		}
	}()
	if engine == nil {
		return RunResult{}, errors.New("script engine is unavailable")
	}
	compiled := program.compiled
	if compiled == nil {
		return RunResult{}, errors.New("script program is not compiled")
	}
	if host == nil {
		return RunResult{}, errors.New("script host is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	if !json.Valid(input) {
		return RunResult{}, errors.New("script input is not valid JSON")
	}
	if err := contextError(ctx); err != nil {
		return RunResult{Failure: &Failure{Kind: "script_cancelled", Message: "Script execution was cancelled.", Path: program.name}}, nil
	}

	runtime := goja.New()
	runtime.SetMaxCallStackSize(engine.config.MaxCallStackSize)
	jsonParse, jsonStringify, freeze, err := runtimeBuiltins(runtime)
	if err != nil {
		return RunResult{}, err
	}
	inputValue, err := parseJSON(runtime, jsonParse, input)
	if err != nil {
		return RunResult{}, fmt.Errorf("prepare script input: %w", err)
	}

	logs := newBoundedLogs(engine.config.MaxOutputBytes)
	var hostErr error
	ctxValue, err := buildContext(runtime, jsonParse, jsonStringify, freeze, host, ctx, logs, &hostErr)
	if err != nil {
		return RunResult{}, err
	}
	watchDone := make(chan struct{})
	watchExited := make(chan struct{})
	go func() {
		defer close(watchExited)
		defer func() { _ = recover() }()
		select {
		case <-ctx.Done():
			runtime.Interrupt(context.Cause(ctx))
		case <-watchDone:
		}
	}()
	defer func() {
		close(watchDone)
		<-watchExited
	}()

	functionValue, runErr := runtime.RunProgram(compiled)
	if runErr != nil {
		return scriptRunFailure(ctx, program.name, runErr, logs.values()), nil
	}
	function, callable := goja.AssertFunction(functionValue)
	if !callable {
		return RunResult{}, errors.New("compiled script did not produce a function")
	}
	value, callErr := function(goja.Undefined(), ctxValue, inputValue)
	if hostErr != nil {
		return RunResult{}, hostErr
	}
	if callErr != nil {
		return scriptRunFailure(ctx, program.name, callErr, logs.values()), nil
	}

	var encoded json.RawMessage
	if goja.IsUndefined(value) {
		encoded = json.RawMessage(`null`)
	} else {
		encoded, err = stringifyJSON(jsonStringify, value)
	}
	if err != nil {
		return RunResult{Logs: logs.values(), Failure: &Failure{
			Kind: "script_result_invalid", Message: err.Error(), Path: program.name,
		}}, nil
	}
	if len(encoded) > engine.config.MaxOutputBytes {
		return RunResult{Logs: logs.values(), Failure: &Failure{
			Kind:    "script_output_limit",
			Message: fmt.Sprintf("Script result exceeds the configured %d byte limit.", engine.config.MaxOutputBytes),
			Path:    program.name,
		}}, nil
	}
	return RunResult{Value: append(json.RawMessage(nil), encoded...), Logs: logs.values()}, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func compileDiagnostic(name string, err error, maxUserLine int) Diagnostic {
	diagnostic := Diagnostic{Kind: "script_compile_failed", Message: err.Error(), Path: name}
	var parseErrors parser.ErrorList
	if errors.As(err, &parseErrors) && len(parseErrors) != 0 && parseErrors[0] != nil {
		diagnostic.Message = parseErrors[0].Message
		diagnostic.Line = min(maxUserLine, max(1, parseErrors[0].Position.Line-wrappedSourceLineOffset))
		diagnostic.Column = parseErrors[0].Position.Column
		return diagnostic
	}
	var syntax *goja.CompilerSyntaxError
	if errors.As(err, &syntax) && syntax != nil {
		diagnostic.Message = syntax.Message
		if syntax.File != nil {
			position := syntax.File.Position(syntax.Offset)
			diagnostic.Line = min(maxUserLine, max(1, position.Line-wrappedSourceLineOffset))
			diagnostic.Column = position.Column
		}
	}
	return diagnostic
}

func scriptRunFailure(ctx context.Context, name string, err error, logs []string) RunResult {
	failure := &Failure{Kind: "script_runtime_failed", Message: err.Error(), Path: name}
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) || contextError(ctx) != nil {
		failure.Kind = "script_cancelled"
		failure.Message = "Script execution was cancelled."
		return RunResult{Logs: logs, Failure: failure}
	}
	var exception *goja.Exception
	if errors.As(err, &exception) && exception != nil {
		frames := exception.Stack()
		for index := range frames {
			position := frames[index].Position()
			if position.Filename != name || position.Line <= wrappedSourceLineOffset {
				continue
			}
			failure.Line = position.Line - wrappedSourceLineOffset
			failure.Column = position.Column
			break
		}
	}
	return RunResult{Logs: logs, Failure: failure}
}

type boundedLogs struct {
	limit     int
	used      int
	truncated bool
	items     []string
}

func newBoundedLogs(limit int) *boundedLogs { return &boundedLogs{limit: limit} }

func (logs *boundedLogs) add(message string) {
	if logs == nil || logs.truncated || logs.limit <= logs.used {
		return
	}
	message = strings.ToValidUTF8(message, "\uFFFD")
	remaining := logs.limit - logs.used
	if len(message) > remaining {
		message = message[:remaining]
		for len(message) > 0 && !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
		logs.truncated = true
	}
	logs.items = append(logs.items, message)
	logs.used += len(message)
}

func (logs *boundedLogs) values() []string {
	if logs == nil {
		return nil
	}
	return append([]string(nil), logs.items...)
}
