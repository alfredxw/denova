package permission_test

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/permission"
	"github.com/alfredxw/denova/agent/permission/permissiontest"
)

func TestCodingPolicyContract(t *testing.T) {
	permissiontest.RunPolicyContract(t, func(testing.TB) agent.PermissionPolicy {
		return permission.Coding(permission.MemoryRules())
	})
}
