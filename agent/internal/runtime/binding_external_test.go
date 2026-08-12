package runtime_test

import runstate "github.com/alfredxw/denova/agent/internal/runtime"

func testBinding(key string) runstate.BindingRef {
	return testBindingAt("/book", key)
}

func testBindingAt(workspace, key string) runstate.BindingRef {
	return runstate.BindingRef{
		Kind: "test", Profile: "default", Key: key,
		Labels: map[string]string{"workspace": workspace},
	}
}

func testBindingWithProfile(workspace, key, profile string) runstate.BindingRef {
	return runstate.BindingRef{
		Kind: "test", Profile: profile, Key: key,
		Labels: map[string]string{"workspace": workspace},
	}
}

func testGameBinding(workspace, story, branch string) runstate.BindingRef {
	return runstate.BindingRef{
		Kind: "test-game", Profile: "default", Key: story + ":" + branch,
		Labels: map[string]string{"workspace": workspace, "story": story, "branch": branch},
	}
}
