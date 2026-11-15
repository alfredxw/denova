package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"denova/internal/book/lore"
	"denova/internal/platform"
	"denova/internal/project"
)

func TestLoreCollectionLifecycle(t *testing.T) {
	ctx := context.Background()
	s := testService(t)
	dir := filepath.Join(s.root, "projects", "collection")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	record, err := s.registry.Add(dir, project.TypeBook, "Collection")
	if err != nil {
		t.Fatal(err)
	}
	store := lore.NewStore(dir)
	if _, err := store.Create(lore.ItemInput{ID: "personal", Name: "Personal", Content: "My notes"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create(lore.ItemInput{ID: "item_0", Name: "Reserved", Content: "Keep this identity"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MutateCategory(lore.CategoryMutation{Op: "create", Name: "Abilities"}); err != nil {
		t.Fatal(err)
	}
	collection := portableCollection[json.RawMessage]{Version: 1, Categories: append(lore.DefaultCategories(), lore.Category{ID: "ability-category", Name: "Abilities"})}
	collection.IndexGuide = &lore.IndexGuide{AutomaticDetails: map[string]string{"auto:ability-category": lore.IndexDetailBrief}, IntroMarkdown: "Imported index introduction", Groups: []lore.IndexGroup{
		{ID: "source-section", Name: "Abilities guide", Purpose: "When choosing an ability", BodyMarkdown: "Imported guide prose", DefaultDetail: lore.IndexDetailBrief},
	}}
	collection.IndexGuide.GroupOrder = []string{"automatic:auto:ability-category", "custom:source-section"}
	collection.IndexGuide.ItemOrder = map[string][]string{"custom:source-section": {"item-2", "item-1", "item-0"}}
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("Item %d", i)
		switch i {
		case 1:
			name = "Item+0" // Different display names can share the same ID base.
		case 2:
			name = "沈凝"
		}
		collection.Items = append(collection.Items, jsonBytes(t, map[string]any{"id": fmt.Sprintf("item-%d", i), "name": name, "type": "ability-category", "content": fmt.Sprintf("Setting %d", i), "index_memberships": []lore.IndexMembership{{GroupID: "source-section", Detail: lore.IndexDetailInherit}}}))
	}
	manifest := Manifest{Format: "denova.resource-pack", SchemaVersion: 1, Package: PackageInfo{ID: "world", Name: "World"}, Resources: []Resource{{ID: "lore", Kind: "lore.collection", Path: "lore.json"}, {ID: "opening", Kind: "game.openings", Path: "opening.json", Requires: []string{"lore"}}}}
	preview := func() Preview {
		t.Helper()
		p, err := s.previewFiles(ctx, Source{Kind: "file", Filename: "world.zip"}, map[string][]byte{"denova-pack.json": jsonBytes(t, manifest), "lore.json": jsonBytes(t, collection), "opening.json": []byte(`{"version":1,"items":[{"id":"arrival","title":"Arrival","content":"You arrive."}]}`)})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := preview()
	if len(p.Candidates[0].Resources) != 2 || p.Candidates[0].Resources[0].ItemCount != 300 {
		t.Fatalf("collection exploded into resources: %+v", p)
	}
	filesPreview, err := s.PreviewFiles(ctx, p.ID, p.Candidates[0].ID, "lore", "lore.json", "item-299")
	if err != nil || len(filesPreview.Items) != 300 || filesPreview.Truncated {
		t.Fatal("collection preview", err)
	}
	var previewItem lore.ItemInput
	if err := json.Unmarshal([]byte(filesPreview.Content), &previewItem); err != nil || previewItem.ID != "item-299" {
		t.Fatal("wrong preview item", err)
	}
	if _, err := s.PreviewFiles(ctx, p.ID, p.Candidates[0].ID, "lore", "lore.json", "not-in-collection"); err == nil {
		t.Fatal("accepted missing item")
	}
	plan, err := s.Plan(ctx, PlanRequest{PreviewID: p.ID, CandidateID: p.Candidates[0].ID, Resources: []string{"opening"}, ProjectID: record.ID})
	if err != nil {
		t.Fatal(err)
	}
	installed, err := s.Apply(ctx, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	var binding Binding
	for _, b := range installed.Bindings {
		if b.Local.Kind == "lore.collection" {
			binding = b
		}
	}
	if len(binding.Members) != 300 {
		t.Fatal("lost member identities")
	}
	if binding.Members["item-0"].ID != "item_0-2" || binding.Members["item-1"].ID != "item_0-3" || binding.Members["item-2"].ID != "沈凝" {
		t.Fatalf("imported IDs must use names and avoid existing identities: %s, %s", binding.Members["item-0"].ID, binding.Members["item-1"].ID)
	}
	items, err := store.ListAll()
	if err != nil || len(items) != 302 {
		t.Fatal(len(items), err)
	}
	importedIndex, err := store.IndexGuide()
	if err != nil || len(importedIndex.Guide.Groups) != 1 || importedIndex.Guide.Groups[0].ID != "source-section" || importedIndex.Guide.AutomaticDetails["auto:ability-category"] != lore.IndexDetailBrief {
		t.Fatalf("lost imported guide: %#v %v", importedIndex, err)
	}
	if fmt.Sprint(importedIndex.Guide.GroupOrder) != fmt.Sprint(collection.IndexGuide.GroupOrder) || fmt.Sprint(importedIndex.Guide.ItemOrder["custom:source-section"]) != "[沈凝 item_0-3 item_0-2]" {
		t.Fatalf("import lost order or failed to translate item IDs: %+v", importedIndex.Guide)
	}
	grouped, err := store.Query(lore.QueryOptions{IndexOptions: lore.IndexOptions{GroupNames: []string{"Abilities guide"}, Limit: 300}})
	if err != nil || len(grouped.Items) != 300 {
		t.Fatalf("lost imported memberships: %d %v", len(grouped.Items), err)
	}
	// Simulate a user edit captured by staging after an earlier live state read.
	// Admission must compare the exact staged snapshot, not read the live file again.
	snapshot, err := os.ReadFile(lore.ItemsPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var edited lore.Collection
	if err := json.Unmarshal(snapshot, &edited); err != nil {
		t.Fatal(err)
	}
	for i := range edited.Items {
		if edited.Items[i].ID == binding.Members["item-0"].ID {
			edited.Items[i].Content = "Edit captured by snapshot"
		}
	}
	staged := map[FileTarget][]byte{{ProjectID: record.ID, Path: lore.ItemsRelativePath}: jsonBytes(t, edited)}
	var extra []FileTarget
	copyBinding := binding
	review := &updateReview{}
	err = s.stageLoreCollection(ctx, "", p.Candidates[0].Resources[0], &copyBinding, jsonBytes(t, collection), review, staged, &extra, map[FileTarget]lore.Asset{})
	if err != nil || review.items[0].State != "keep" {
		t.Fatalf("staged edit not protected: %+v %v", review.items, err)
	}
	exported, err := s.Export(ctx, ExportRequest{Package: installed.Package, InstallationID: installed.ID, Resources: []LocalRef{binding.Local}})
	if err != nil {
		t.Fatal(err)
	}
	files, err := platform.ArchiveFiles(exported)
	if err != nil || len(files) != 2 {
		t.Fatalf("expected only manifest and collection: %d %v", len(files), err)
	}
	exportedGuideFound := false
	for name, data := range files {
		if name == "denova-pack.json" {
			continue
		}
		exportedCollection, exportedItems, err := readLoreCollection(data)
		if err != nil {
			t.Fatal(err)
		}
		exportedGuideFound = exportedCollection.IndexGuide != nil && len(exportedCollection.IndexGuide.Groups) == 1 && len(exportedItems[0].IndexMemberships) == 1
	}
	if !exportedGuideFound {
		t.Fatal("export discarded the guide or memberships")
	}
	var roundtrip Manifest
	if err := json.Unmarshal(files["denova-pack.json"], &roundtrip); err != nil {
		t.Fatal(err)
	}
	portable, portableItems, err := readLoreCollection(files[roundtrip.Resources[0].Path])
	if err != nil || len(portableItems) != 300 || portableItems[0].ID != "item-0" {
		t.Fatal("roundtrip identities", len(portableItems), err)
	}
	if fmt.Sprint(portable.IndexGuide.ItemOrder["custom:source-section"]) != "[item-2 item-1 item-0]" {
		t.Fatalf("export lost ordered package identities: %+v", portable.IndexGuide.ItemOrder)
	}
	category, err := store.Categories()
	if err != nil {
		t.Fatal(err)
	}
	if len(category) != 7 || category[6].ID != "ability-category" || category[6].Name != "Abilities (2)" {
		t.Fatalf("category collision lost identity: %+v", category)
	}
	if len(portable.Categories) != 2 || portable.Categories[1] != category[6] || portableItems[0].Type != category[6].ID {
		t.Fatalf("category roundtrip lost definition: %+v", portable.Categories)
	}
	if _, err := s.Export(ctx, ExportRequest{Package: installed.Package, Resources: []LocalRef{binding.Local, {Kind: "lore.collection", Scope: "project", ProjectID: record.ID, ID: "all"}}}); err == nil {
		t.Fatal("export accepted overlapping Lore collections")
	}
	var openingRef LocalRef
	for _, b := range installed.Bindings {
		if b.Local.Kind == "game.openings" {
			openingRef = b.Local
		}
	}
	withDependencies, err := s.Export(ctx, ExportRequest{Package: installed.Package, InstallationID: installed.ID, Resources: []LocalRef{openingRef}})
	if err != nil {
		t.Fatal(err)
	}
	dependentFiles, err := platform.ArchiveFiles(withDependencies)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(dependentFiles["denova-pack.json"], &roundtrip); err != nil {
		t.Fatal(err)
	}
	if len(roundtrip.Resources) != 2 {
		t.Fatal("export lost collection dependency")
	}
	for _, resource := range roundtrip.Resources {
		if resource.Kind == "game.openings" && (len(resource.Requires) != 1 || resource.Requires[0] != "lore") {
			t.Fatal("export changed dependency identity", resource)
		}
	}
	// Unrelated project edits must not block a collection update.
	if _, err := store.Update("personal", lore.ItemInput{Name: "Personal", Content: "Changed independently"}); err != nil {
		t.Fatal(err)
	}
	collection.Items = collection.Items[:299]
	collection.Items[0] = jsonBytes(t, map[string]any{"id": "item-0", "name": "Renamed Item Zero", "type": "world", "content": "Updated setting"})
	p = preview()
	request := PlanRequest{PreviewID: p.ID, CandidateID: p.Candidates[0].ID, Resources: []string{"lore"}, InstallationID: installed.ID}
	updated, err := s.Plan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, updated.ID); err != nil {
		t.Fatal(err)
	}
	item, err := store.ReadAny(binding.Members["item-0"].ID)
	if err != nil || item.Content != "Updated setting" || item.Name != "Renamed Item Zero" || item.ID != "item_0-2" {
		t.Fatal(item, err)
	}
	if _, err := store.ReadAny(binding.Members["item-299"].ID); err != nil {
		t.Fatal("upstream removal deleted local content", err)
	}
	if _, err := store.Update(item.ID, lore.ItemInput{Name: item.Name, Content: "Personal edit"}); err != nil {
		t.Fatal(err)
	}
	collection.Items[0] = jsonBytes(t, map[string]any{"id": "item-0", "name": "Item 0", "type": "world", "content": "Another upstream edit"})
	p = preview()
	request.PreviewID = p.ID
	protected, err := s.Plan(ctx, request)
	if err != nil || protected.Updates[0].State != "conflict" {
		t.Fatalf("local edit not protected: %v", err)
	}
	request.Resolutions = map[string]map[string]string{"lore": {"item-0": "remote"}}
	replaced, err := s.Plan(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, replaced.ID); err != nil {
		t.Fatal(err)
	}
	restore, err := s.PlanRestore(ctx, replaced.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, restore.ID); err != nil {
		t.Fatal(err)
	}
	item, err = store.ReadAny(item.ID)
	if err != nil || item.Content != "Personal edit" {
		t.Fatal("rollback lost edits", item, err)
	}
}

func TestLoreCollectionRejectsInvalidMembers(t *testing.T) {
	for _, raw := range []string{
		`{"version":1,"items":[]}`,
		`{"version":1,"items":[{"id":"../x","name":"Invalid ID"}]}`,
		`{"version":2,"items":[{"id":"a","name":"A"}]}`,
		`{"version":1,"items":[{"id":"a","name":"A"},{"id":"a","name":"B"}]}`,
		`{"version":1,"items":[{"id":"a","name":"A"},{"id":"b","name":"A"}]}`,
		`{"version":1,"items":[{"id":"a","name":"A","image":{"image_path":"/private/host.png"}}]}`,
	} {
		if err := validatePayload("lore.collection", []byte(raw)); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
}

func TestCollectionAcceptsNativeItemIdentitiesAndLargeCharacterCards(t *testing.T) {
	// IDs are data inside the collection, not filenames or manifest resource IDs.
	if _, _, err := readLoreCollection([]byte(`{"version":1,"items":[{"id":"人物-甲","name":"甲","content":"A character."}]}`)); err != nil {
		t.Fatal(err)
	}
	entries := make([]map[string]any, 300)
	for i := range entries {
		entries[i] = map[string]any{"id": i, "keys": []string{fmt.Sprintf("place-%d", i)}, "comment": fmt.Sprintf("Place %d", i), "content": "An independently readable place.", "enabled": true}
	}
	raw := jsonBytes(t, map[string]any{"spec": "chara_card_v2", "data": map[string]any{"name": "Large world", "description": "A world guide.", "character_book": map[string]any{"entries": entries}}})
	s := testService(t)
	p, err := s.Preview(context.Background(), Source{Kind: "file", Filename: "world.json"}, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Candidates) != 1 || len(p.Candidates[0].Resources) != 1 || p.Candidates[0].Resources[0].Kind != "lore.collection" || p.Candidates[0].Resources[0].ItemCount < 300 {
		t.Fatalf("character conversion split the collection: %+v", p)
	}
}
