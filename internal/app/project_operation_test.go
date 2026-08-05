package app

import (
	"context"
	"testing"

	"denova/config"
)

func TestAcquireProjectOperationBorrowsMatchingRequestScope(t *testing.T) {
	root := t.TempDir()
	application, err := New(context.Background(), &config.Config{
		OpenAIModel:         "test-model",
		NovaDir:             root,
		Workspace:           root,
		ResumeLastWorkspace: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(application.Close)

	owner, err := application.AcquireProjectOperation(context.Background(), application.ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	borrowed, err := application.AcquireProjectOperation(owner.Context(), application.ProjectID())
	if err != nil {
		t.Fatal(err)
	}
	if borrowed.lease != nil {
		t.Fatal("nested Project operation acquired a duplicate lifecycle lease")
	}
	if borrowed.Layout() != owner.Layout() {
		t.Fatalf("borrowed Project layout changed: owner=%#v borrowed=%#v", owner.Layout(), borrowed.Layout())
	}
	borrowed.Release()
	if err := owner.Context().Err(); err != nil {
		t.Fatalf("releasing a borrowed scope canceled its request owner: %v", err)
	}
}
