package script

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/dop251/goja"
)

func runtimeBuiltins(runtime *goja.Runtime) (
	jsonParse goja.Callable,
	jsonStringify goja.Callable,
	freeze goja.Callable,
	err error,
) {
	jsonObject := runtime.Get("JSON").ToObject(runtime)
	jsonParse, ok := goja.AssertFunction(jsonObject.Get("parse"))
	if !ok {
		return nil, nil, nil, errors.New("JavaScript JSON.parse is unavailable")
	}
	jsonStringify, ok = goja.AssertFunction(jsonObject.Get("stringify"))
	if !ok {
		return nil, nil, nil, errors.New("JavaScript JSON.stringify is unavailable")
	}
	object := runtime.Get("Object").ToObject(runtime)
	freeze, ok = goja.AssertFunction(object.Get("freeze"))
	if !ok {
		return nil, nil, nil, errors.New("JavaScript Object.freeze is unavailable")
	}
	return jsonParse, jsonStringify, freeze, nil
}

func buildContext(
	runtime *goja.Runtime,
	jsonParse goja.Callable,
	jsonStringify goja.Callable,
	freeze goja.Callable,
	host Host,
	hostContext context.Context,
	logs *boundedLogs,
	hostErr *error,
) (goja.Value, error) {
	tools := runtime.NewObject()
	if err := tools.SetPrototype(nil); err != nil {
		return nil, fmt.Errorf("create script tools object: %w", err)
	}
	call := func(call goja.FunctionCall) goja.Value {
		request, invalid := decodeCall(runtime, jsonStringify, call.Argument(0), call.Argument(1))
		if invalid != nil {
			return mustParseJSON(runtime, jsonParse, mustMarshal(invalidOutcome(request.Name, invalid.Error())))
		}
		outcomes, err := host.CallTools(hostContext, []Call{request})
		if err != nil {
			*hostErr = err
			panic(runtime.NewGoError(err))
		}
		if len(outcomes) != 1 {
			err := fmt.Errorf("script host returned %d outcomes for one call", len(outcomes))
			*hostErr = err
			panic(runtime.NewGoError(err))
		}
		return mustParseJSON(runtime, jsonParse, mustMarshal(outcomes[0]))
	}
	parallel := func(call goja.FunctionCall) goja.Value {
		requests, slots, err := decodeParallel(runtime, jsonStringify, call.Argument(0))
		if err != nil {
			return mustParseJSON(runtime, jsonParse, mustMarshal([]Outcome{invalidOutcome("", err.Error())}))
		}
		outcomes, callErr := host.CallTools(hostContext, requests)
		if callErr != nil {
			*hostErr = callErr
			panic(runtime.NewGoError(callErr))
		}
		if len(outcomes) != len(requests) {
			err := fmt.Errorf("script host returned %d outcomes for %d calls", len(outcomes), len(requests))
			*hostErr = err
			panic(runtime.NewGoError(err))
		}
		for index := range slots {
			if slots[index].invalid == nil {
				slots[index].outcome = outcomes[slots[index].requestIndex]
			}
		}
		ordered := make([]Outcome, len(slots))
		for index := range slots {
			ordered[index] = slots[index].outcome
		}
		return mustParseJSON(runtime, jsonParse, mustMarshal(ordered))
	}
	if err := tools.DefineDataProperty("call", runtime.ToValue(call), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define ctx.tools.call: %w", err)
	}
	if err := tools.DefineDataProperty("parallel", runtime.ToValue(parallel), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define ctx.tools.parallel: %w", err)
	}
	if _, err := freeze(goja.Undefined(), tools); err != nil {
		return nil, fmt.Errorf("freeze ctx.tools: %w", err)
	}

	contextObject := runtime.NewObject()
	if err := contextObject.SetPrototype(nil); err != nil {
		return nil, fmt.Errorf("create script context object: %w", err)
	}
	if err := contextObject.DefineDataProperty("tools", tools, goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define ctx.tools: %w", err)
	}
	log := func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		if goja.IsUndefined(value) || goja.IsNull(value) {
			logs.add("")
			return goja.Undefined()
		}
		if _, callable := goja.AssertFunction(value); callable {
			panic(runtime.NewTypeError("ctx.log(message) requires a string"))
		}
		logs.add(value.String())
		return goja.Undefined()
	}
	if err := contextObject.DefineDataProperty("log", runtime.ToValue(log), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		return nil, fmt.Errorf("define ctx.log: %w", err)
	}
	if _, err := freeze(goja.Undefined(), contextObject); err != nil {
		return nil, fmt.Errorf("freeze script context: %w", err)
	}
	return contextObject, nil
}

type outcomeSlot struct {
	requestIndex int
	invalid      error
	outcome      Outcome
}

func decodeParallel(runtime *goja.Runtime, stringify goja.Callable, value goja.Value) ([]Call, []outcomeSlot, error) {
	object, ok := value.(*goja.Object)
	if !ok || object.ClassName() != "Array" {
		return nil, nil, errors.New("ctx.tools.parallel(calls) requires an array")
	}
	length := object.Get("length").ToInteger()
	if length < 0 || length > math.MaxInt32 {
		return nil, nil, errors.New("ctx.tools.parallel(calls) has an invalid length")
	}
	requests := make([]Call, 0, length)
	slots := make([]outcomeSlot, int(length))
	for index := range slots {
		item := object.Get(fmt.Sprintf("%d", index))
		itemObject, valid := item.(*goja.Object)
		if !valid || itemObject.ClassName() != "Object" {
			slots[index].invalid = errors.New("parallel item must be {tool, input}")
			slots[index].outcome = invalidOutcome("", slots[index].invalid.Error())
			continue
		}
		request, invalid := decodeCall(runtime, stringify, itemObject.Get("tool"), itemObject.Get("input"))
		if invalid != nil {
			slots[index].invalid = invalid
			slots[index].outcome = invalidOutcome(request.Name, invalid.Error())
			continue
		}
		slots[index].requestIndex = len(requests)
		requests = append(requests, request)
	}
	return requests, slots, nil
}

func decodeCall(runtime *goja.Runtime, stringify goja.Callable, nameValue, inputValue goja.Value) (Call, error) {
	name, ok := nameValue.Export().(string)
	name = strings.TrimSpace(name)
	request := Call{Name: name}
	if !ok || name == "" {
		return request, errors.New("tool name must be a non-empty string")
	}
	if goja.IsUndefined(inputValue) {
		inputValue = runtime.NewObject()
	}
	encoded, err := stringifyJSON(stringify, inputValue)
	if err != nil {
		return request, fmt.Errorf("tool input is not JSON-compatible: %w", err)
	}
	request.Arguments = encoded
	return request, nil
}

func stringifyJSON(stringify goja.Callable, value goja.Value) (json.RawMessage, error) {
	if _, callable := goja.AssertFunction(value); callable {
		return nil, errors.New("functions are not JSON-compatible")
	}
	if object, ok := value.(*goja.Object); ok && object.ExportType() == reflect.TypeOf((*goja.Promise)(nil)) {
		return nil, errors.New("Promise values are not supported")
	}
	encoded, err := stringify(goja.Undefined(), value)
	if err != nil {
		return nil, err
	}
	if goja.IsUndefined(encoded) {
		return nil, errors.New("value is not JSON-compatible")
	}
	text, ok := encoded.Export().(string)
	if !ok || !json.Valid([]byte(text)) {
		return nil, errors.New("JSON.stringify returned an invalid value")
	}
	return json.RawMessage(text), nil
}

func invalidOutcome(name, message string) Outcome {
	return Outcome{
		Tool: name, OK: false, Status: "error", Output: mustMarshal(message), Reason: "invalid_arguments",
	}
}

func mustMarshal(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func parseJSON(runtime *goja.Runtime, parse goja.Callable, raw json.RawMessage) (goja.Value, error) {
	return parse(goja.Undefined(), runtime.ToValue(string(raw)))
}

func mustParseJSON(runtime *goja.Runtime, parse goja.Callable, raw json.RawMessage) goja.Value {
	value, err := parseJSON(runtime, parse, raw)
	if err != nil {
		panic(runtime.NewGoError(err))
	}
	return value
}
