package session_test

import (
	"testing"

	"github.com/alfredxw/denova/agent/session"
	"github.com/alfredxw/denova/agent/session/sessiontest"
)

func TestMemoryStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(testing.TB) session.Store {
		return session.Memory()
	})
}
