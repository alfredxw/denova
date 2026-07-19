package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestMutateSettingsFileAllowsOnlyOneWriterForSameRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, Settings{OpenAIModel: "base"}); err != nil {
		t.Fatal(err)
	}
	baseRevision, err := SettingsFileRevision(path)
	if err != nil {
		t.Fatal(err)
	}

	models := []string{"first", "second"}
	errs := make([]error, len(models))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for index, model := range models {
		wg.Add(1)
		go func(index int, model string) {
			defer wg.Done()
			defer captureSettingsMutationPanic(&errs[index])
			<-start
			_, errs[index] = MutateSettingsFile(path, baseRevision, func(current Settings) (Settings, error) {
				current.OpenAIModel = model
				return current, nil
			})
		}(index, model)
	}
	close(start)
	wg.Wait()

	succeeded := 0
	conflicted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrSettingsRevisionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected mutation error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent settings results: succeeded=%d conflicted=%d errors=%v", succeeded, conflicted, errs)
	}

	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.OpenAIModel != "first" && settings.OpenAIModel != "second" {
		t.Fatalf("unexpected persisted model %q", settings.OpenAIModel)
	}
}

func TestMutateSettingsFileSerializesBlindReadModifyWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteSettingsFile(path, Settings{OpenAIModel: "base", Theme: "dark"}); err != nil {
		t.Fatal(err)
	}

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer captureSettingsMutationPanic(&errs[0])
		_, errs[0] = MutateSettingsFile(path, "", func(current Settings) (Settings, error) {
			close(firstEntered)
			<-releaseFirst
			current.OpenAIModel = "first"
			return current, nil
		})
	}()
	<-firstEntered
	go func() {
		defer wg.Done()
		defer captureSettingsMutationPanic(&errs[1])
		close(secondStarted)
		_, errs[1] = MutateSettingsFile(path, "", func(current Settings) (Settings, error) {
			current.Theme = "light"
			return current, nil
		})
	}()
	<-secondStarted
	close(releaseFirst)
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("blind mutation failed: %v", err)
		}
	}

	settings, err := ReadSettingsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.OpenAIModel != "first" || settings.Theme != "light" {
		t.Fatalf("serialized mutations lost a field: %#v", settings)
	}
}

func captureSettingsMutationPanic(target *error) {
	if recovered := recover(); recovered != nil {
		*target = fmt.Errorf("settings mutation panic: %v", recovered)
	}
}
