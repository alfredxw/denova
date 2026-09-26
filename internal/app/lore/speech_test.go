package loreapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"denova/config"
	booklore "denova/internal/book/lore"
	"denova/internal/speech"
)

type speechTestHost struct{ store *booklore.Store }

func (host speechTestHost) WithLoreStore(_ context.Context, _ string, action func(*booklore.Store) error) (string, error) {
	return "", action(host.store)
}
func (speechTestHost) ClassifyLoreItems(context.Context, string, []booklore.ClassificationInput) ([]booklore.ClassificationSuggestion, error) {
	return nil, errors.New("classification is not used by speech generation")
}

func TestGenerateSpeechAppendsValidatedAudioWithoutChangingLoreOnFailure(t *testing.T) {
	workspace := t.TempDir()
	store := booklore.NewStore(workspace)
	if _, err := store.Create(booklore.ItemInput{ID: "hero", Name: "Hero", Type: "character", Content: "Keep this setting"}); err != nil {
		t.Fatal(err)
	}
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0xc0})
	mp3 := bytes.Repeat(frame, 4)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]string
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input["model"] != "tts" || input["voice"] != "narrator" || input["response_format"] != "mp3" {
			t.Errorf("unexpected speech request: %#v", input)
		}
		switch input["input"] {
		case "Denied":
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("private provider detail"))
		case "Invalid":
			w.Write([]byte("ID3truncated"))
		default:
			w.Write(mp3)
		}
	}))
	defer provider.Close()
	settings := config.SpeechSettings{Endpoint: provider.URL, Model: "tts", Voice: "narrator"}
	service := NewService(speechTestHost{store}, nil)
	for _, text := range []string{"Hello.", "Welcome."} {
		item, err := service.GenerateItemSpeech(context.Background(), "project", "hero", ItemSpeechGenerateRequest{Name: "Greeting", Text: text}, settings)
		if err != nil {
			t.Fatal(err)
		}
		material := item.ResolvedMaterials[len(item.ResolvedMaterials)-1]
		if item.Content != "Keep this setting" || item.Image != nil || material.Name != "Greeting" || material.Description != text || material.Source.Kind != "generated" || material.MIMEType != "audio/mpeg" {
			t.Fatalf("incorrect generated material: %#v", item)
		}
		data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(material.Path)))
		if err != nil || !bytes.Equal(data, mp3) {
			t.Fatalf("generated audio missing or changed: %v", err)
		}
	}
	item, err := store.ReadAny("hero")
	if err != nil || len(item.ResolvedMaterials) != 2 || item.ResolvedMaterials[0].Path == item.ResolvedMaterials[1].Path {
		t.Fatalf("generation did not append immutable files: %#v, %v", item, err)
	}
	metadataPath := filepath.Join(workspace, "setting/lore/items.json")
	before, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		ctx  context.Context
		text string
		want error
	}{
		{context.Background(), "Denied", speech.Auth},
		{context.Background(), "Invalid", speech.Audio},
		{context.Background(), "", speech.InvalidInput},
		{canceled, "Canceled", context.Canceled},
	} {
		_, err := service.GenerateItemSpeech(tc.ctx, "project", "hero", ItemSpeechGenerateRequest{Text: tc.text}, settings)
		if !errors.Is(err, tc.want) {
			t.Fatalf("input %q: got %v, want %v", tc.text, err, tc.want)
		}
	}
	after, err := os.ReadFile(metadataPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed generation changed lore: %v", err)
	}
	files, err := os.ReadDir(filepath.Join(workspace, "assets/lore/media"))
	if err != nil || len(files) != 2 {
		t.Fatalf("failed generation leaked files: %v, %v", files, err)
	}
}
