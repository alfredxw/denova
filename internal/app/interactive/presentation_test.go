package interactiveapp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/internal/book/lore"
	"denova/internal/interactive"
)

func presentationLoreFixture(t *testing.T, workspace string, id string) lore.Material {
	t.Helper()
	store := lore.NewStore(workspace)
	if _, err := store.Create(lore.ItemInput{ID: id, Name: id, Type: "character", BriefDescription: "An investigator", Content: "Full lore body must not enter the material catalog."}); err != nil {
		t.Fatal(err)
	}
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	item, err := store.UploadMaterial(t.Context(), id, "happy.png", imageData.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return item.ResolvedMaterials[0]
}

func TestPresentationCatalogAndResolverUseEnabledAssociatedImages(t *testing.T) {
	workspace := t.TempDir()
	material := presentationLoreFixture(t, workspace, "hero")
	raw := json.RawMessage(fmt.Sprintf(`{"characters":[{"item_id":"hero","asset_id":%q}]}`, material.ID))
	stage, receipt := resolvePresentationPatch(workspace, nil, raw, nil)
	if receipt.Applied != 1 || stage.Characters[0].Path != material.Path {
		t.Fatalf("resolve=%#v %#v", stage, receipt)
	}
	source := buildPresentationContext(workspace, nil, stage, nil, "hero")
	if !strings.Contains(source.Content, material.ID) || !strings.Contains(source.Content, "An investigator") || strings.Contains(source.Content, "Full lore body") || source.Limit != 64*1024 {
		t.Fatalf("catalog=%s", source.Content)
	}
	if again := buildPresentationContext(workspace, nil, stage, nil, "hero"); again.Content != source.Content {
		t.Fatal("catalog order is unstable")
	}
	// Removing an association makes new selections invalid without changing a
	// committed stage's concrete locator.
	if _, err := lore.NewStore(workspace).MutateMaterial("hero", lore.MaterialMutation{Op: "remove", AssetID: material.ID}); err != nil {
		t.Fatal(err)
	}
	preserved, receipt := resolvePresentationPatch(workspace, stage, raw, nil)
	if receipt.Ignored != 1 || !reflect.DeepEqual(preserved, stage) {
		t.Fatal("missing association erased an existing sprite")
	}
	if err := os.Remove(filepath.Join(workspace, filepath.FromSlash(material.Path))); err != nil {
		t.Fatal(err)
	}
	disabled := buildPresentationContext(workspace, &interactive.StoryPresentationSettings{}, stage, nil, "hero")
	if strings.Contains(disabled.Content, "image catalog") || !strings.Contains(disabled.Content, "Both layers are disabled") {
		t.Fatal("disabled layers injected a catalog")
	}
}

func TestPresentationCatalogSkipsOversizeCompleteItems(t *testing.T) {
	workspace := t.TempDir()
	material := presentationLoreFixture(t, workspace, "oversize")
	if _, err := lore.NewStore(workspace).MutateMaterial("oversize", lore.MaterialMutation{Op: "update", AssetID: material.ID, Description: strings.Repeat("x", presentationContextMaxBytes)}); err != nil {
		t.Fatal(err)
	}
	presentationLoreFixture(t, workspace, "small")
	source := buildPresentationContext(workspace, nil, nil, nil, "oversize")
	if len(source.Content) > presentationContextMaxBytes || !source.Truncated || !strings.Contains(source.Content, `"item_id":"small"`) || strings.Contains(source.Content, `"item_id":"oversize"`) || !strings.Contains(source.Content, "1 items (1 images) omitted") {
		t.Fatalf("bounded catalog=%s", source.Content)
	}
}

func TestPresentationSubmissionRecoversAcceptedStageAndIgnoresBadReferences(t *testing.T) {
	workspace := t.TempDir()
	material := presentationLoreFixture(t, workspace, "hero")
	store := interactive.NewStore(workspace)
	defer store.Close()
	story, err := store.CreateStory(interactive.CreateStoryRequest{Title: "Stage recovery", PlanningMode: interactive.StoryPlanningModeDisabled})
	if err != nil {
		t.Fatal(err)
	}
	c := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "Enter", 800, nil)
	bindInteractiveCycleForTest(t, c)
	args := strings.TrimSuffix(gameStateArgs, "}") + fmt.Sprintf(`,"presentation":{"background":{"item_id":"missing","asset_id":"bad"},"characters":[{"item_id":"hero","asset_id":%q}]}}`, material.ID)
	receipt, err := c.SubmitTurnResult(t.Context(), interactive.DecodeInteractiveTurnSubmissionInput(args))
	if err != nil || receipt.Ready || receipt.Presentation.Applied != 1 || receipt.Presentation.Ignored != 1 || !reflect.DeepEqual(receipt.RetryModules, []string{"choices"}) {
		t.Fatalf("partial receipt=%#v err=%v", receipt, err)
	}
	restored := NewConversation(store, t.TempDir(), workspace, story.ID, "main", "Enter", 800, nil)
	restored.BindAgentCycleIdentity(c.AgentCycleIdentitySnapshot())
	receipt, err = restored.SubmitTurnResult(t.Context(), interactive.DecodeInteractiveTurnSubmissionInput(`{"choices":["Enter","Observe","Listen","Inspect","Wait"],"presentation":{"characters":[{"item_id":"hero","asset_id":"typo"}]}}`))
	if err != nil || !receipt.Ready || len(receipt.RetryModules) != 0 || receipt.Presentation.Ignored != 1 {
		t.Fatalf("visual error blocked readiness: %#v %v", receipt, err)
	}
	if err := commitInteractiveAssistantForTest(t, restored, "The investigator arrives.", ""); err != nil {
		t.Fatal(err)
	}
	snapshot, err := interactive.NewStore(workspace).Snapshot(story.ID, "main")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CurrentTurn == nil || snapshot.CurrentTurn.TurnResult.Presentation.Characters[0].AssetID != material.ID {
		t.Fatal("recovered turn lost accepted sprite")
	}
}
