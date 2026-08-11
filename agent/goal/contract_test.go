package goal_test

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/goal"
	"github.com/alfredxw/denova/agent/goal/goaltest"
)

func TestStandardManagerContract(t *testing.T) {
	goaltest.RunManagerContract(t, func(testing.TB) agent.GoalManager {
		return goal.Standard()
	})
}
