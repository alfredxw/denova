package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestScopeCloseFencesAndWaitsForLease(t *testing.T) {
	root := NewRoot("app")
	workspace, err := root.Child("workspace")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := workspace.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	workspace.BeginClose()
	if _, err := workspace.Acquire(); !errors.Is(err, ErrClosing) {
		t.Fatalf("Acquire after fence error = %v, want ErrClosing", err)
	}
	select {
	case <-workspace.done:
		t.Fatal("scope closed before lease release")
	default:
	}
	lease.Release()
	if err := workspace.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Acquire(); !errors.Is(err, ErrClosed) {
		t.Fatalf("Acquire after close error = %v, want ErrClosed", err)
	}
}

func TestRootCloseWaitsForChildLease(t *testing.T) {
	root := NewRoot("app")
	workspace, err := root.Child("workspace")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := workspace.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	root.BeginClose()
	if _, err := root.Child("late"); !errors.Is(err, ErrClosing) {
		t.Fatalf("Child after fence error = %v, want ErrClosing", err)
	}
	select {
	case <-root.done:
		t.Fatal("root closed before child lease release")
	default:
	}
	lease.Release()
	if err := root.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
