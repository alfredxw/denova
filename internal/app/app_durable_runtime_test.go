package app

import (
	"context"
	"errors"
	"testing"

	"denova/config"
)

func TestNewRequiresDurableAgentDataDirectory(t *testing.T) {
	application, err := New(context.Background(), &config.Config{})
	if application != nil {
		application.Close()
		t.Fatal("App.New returned an application backed by an ephemeral agent runtime")
	}
	if !errors.Is(err, ErrAgentDataDirRequired) {
		t.Fatalf("App.New error = %v, want ErrAgentDataDirRequired", err)
	}
}

func TestNewRejectsNilConfig(t *testing.T) {
	application, err := New(context.Background(), nil)
	if application != nil {
		application.Close()
		t.Fatal("App.New returned an application for a nil config")
	}
	if !errors.Is(err, ErrAgentDataDirRequired) {
		t.Fatalf("App.New nil config error = %v, want ErrAgentDataDirRequired", err)
	}
}
