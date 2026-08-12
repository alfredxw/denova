package file_test

import (
	"testing"

	"github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
	"github.com/alfredxw/denova/agent/session/sessiontest"
)

func TestPublicStoreContract(t *testing.T) {
	sessiontest.RunStoreContract(t, func(t testing.TB) session.Store {
		store, err := sessionfile.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}
