package agents

import runstate "github.com/alfredxw/denova/agent/runtime"

func mustRuntimeBinding(binding RuntimeBinding) runstate.BindingRef {
	ref, err := binding.Ref()
	if err != nil {
		panic(err)
	}
	return ref
}
