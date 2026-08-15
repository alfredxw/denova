package script

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type hostFunc func(context.Context, []Call) ([]Outcome, error)

func (function hostFunc) CallTools(ctx context.Context, calls []Call) ([]Outcome, error) {
	return function(ctx, calls)
}

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := NewEngine(Config{MaxSourceBytes: 64 << 10, MaxOutputBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func compileTestProgram(t *testing.T, engine *Engine, code string) Program {
	t.Helper()
	program, diagnostics := engine.Compile(context.Background(), Source{Name: "tool.js", Code: code})
	if len(diagnostics) != 0 {
		t.Fatalf("compile diagnostics: %+v", diagnostics)
	}
	return program
}

func TestEngineRunsFunctionBodyWithIsolatedInput(t *testing.T) {
	engine := newTestEngine(t)
	program := compileTestProgram(t, engine, `
ctx.log("starting")
input.value += 1
return {value: input.value, hasMain: typeof main !== "undefined"}
`)
	result, err := engine.Run(nil, program, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
		t.Fatal("host should not be called")
		return nil, nil
	}), json.RawMessage(`{"value":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure != nil {
		t.Fatalf("unexpected failure: %+v", result.Failure)
	}
	if string(result.Value) != `{"value":8,"hasMain":false}` {
		t.Fatalf("value = %s", result.Value)
	}
	if len(result.Logs) != 1 || result.Logs[0] != "starting" {
		t.Fatalf("logs = %#v", result.Logs)
	}
}

func TestEngineCallAndParallelPreserveOrderAndPartialInvalidItems(t *testing.T) {
	engine := newTestEngine(t)
	program := compileTestProgram(t, engine, `
const one = ctx.tools.call("read", {path: "one"})
const many = ctx.tools.parallel([
  {tool: "read", input: {path: "two"}},
  {tool: "", input: {}},
  {tool: "read", input: {path: "three"}}
])
return {one, many}
`)
	var batches [][]string
	host := hostFunc(func(_ context.Context, calls []Call) ([]Outcome, error) {
		names := make([]string, len(calls))
		outcomes := make([]Outcome, len(calls))
		for index, call := range calls {
			names[index] = call.Name
			outcomes[index] = Outcome{
				Tool: call.Name, OK: true, Status: "success", Output: append(json.RawMessage(nil), call.Arguments...),
			}
		}
		batches = append(batches, names)
		return outcomes, nil
	})
	result, err := engine.Run(context.Background(), program, host, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure != nil {
		t.Fatalf("unexpected failure: %+v", result.Failure)
	}
	if len(batches) != 2 || len(batches[0]) != 1 || len(batches[1]) != 2 {
		t.Fatalf("host batches = %#v", batches)
	}
	var value struct {
		Many []Outcome `json:"many"`
	}
	if err := json.Unmarshal(result.Value, &value); err != nil {
		t.Fatal(err)
	}
	if len(value.Many) != 3 || value.Many[0].OK != true || value.Many[1].Reason != "invalid_arguments" || value.Many[2].OK != true {
		t.Fatalf("parallel outcomes = %+v", value.Many)
	}
}

func TestEngineReportsUserLineForCompileAndRuntimeFailures(t *testing.T) {
	engine := newTestEngine(t)
	_, diagnostics := engine.Compile(context.Background(), Source{Name: "saved.js", Code: "const ok = 1\nreturn )"})
	if len(diagnostics) != 1 || diagnostics[0].Line != 2 || diagnostics[0].Path != "saved.js" {
		t.Fatalf("compile diagnostics = %+v", diagnostics)
	}
	program := compileTestProgram(t, engine, "const ok = 1\nthrow new Error('broken')")
	result, err := engine.Run(context.Background(), program, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
		return nil, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure == nil || result.Failure.Kind != "script_runtime_failed" || result.Failure.Line != 2 {
		t.Fatalf("runtime failure = %+v", result.Failure)
	}
}

func TestEngineRejectsNonJSONResults(t *testing.T) {
	engine := newTestEngine(t)
	for _, code := range []string{
		"return function () {}",
		"return Promise.resolve(1)",
		"const value = {}; value.self = value; return value",
	} {
		program := compileTestProgram(t, engine, code)
		result, err := engine.Run(context.Background(), program, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
			return nil, nil
		}), nil)
		if err != nil {
			t.Fatalf("code %q: %v", code, err)
		}
		if result.Failure == nil || result.Failure.Kind != "script_result_invalid" {
			t.Fatalf("code %q failure = %+v", code, result.Failure)
		}
	}
}

func TestEngineUsesStandardJSONStringifySemantics(t *testing.T) {
	engine := newTestEngine(t)
	program := compileTestProgram(t, engine, "return {number: NaN, omitted: undefined, array: [undefined]}")
	result, err := engine.Run(context.Background(), program, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
		return nil, nil
	}), nil)
	if err != nil || result.Failure != nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if string(result.Value) != `{"number":null,"array":[null]}` {
		t.Fatalf("value = %s", result.Value)
	}
}

func TestEngineInterruptsTightLoopAndPropagatesHostFailure(t *testing.T) {
	engine := newTestEngine(t)
	loop := compileTestProgram(t, engine, "for (;;) {}")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)
	result, err := engine.Run(ctx, loop, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
		return nil, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure == nil || result.Failure.Kind != "script_cancelled" {
		t.Fatalf("cancel failure = %+v", result.Failure)
	}

	hostFailure := errors.New("host unavailable")
	call := compileTestProgram(t, engine, `return ctx.tools.call("read", {})`)
	_, err = engine.Run(context.Background(), call, hostFunc(func(context.Context, []Call) ([]Outcome, error) {
		return nil, hostFailure
	}), nil)
	if !errors.Is(err, hostFailure) && (err == nil || !strings.Contains(err.Error(), hostFailure.Error())) {
		t.Fatalf("host error = %v", err)
	}
}
