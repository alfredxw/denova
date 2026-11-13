package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"denova/internal/book/lore"
	"denova/internal/revisionfile"
)

func readLoreCollection(raw []byte) (portableCollection[json.RawMessage], []lore.Item, error) {
	var collection portableCollection[json.RawMessage]
	if err := decode(raw, &collection); err != nil {
		return collection, nil, err
	}
	if collection.Version != 1 || len(collection.Items) == 0 {
		return collection, nil, fmt.Errorf("Lore collection requires version 1 and nonempty items")
	}
	native := lore.Collection{Version: 3, Categories: collection.Categories, Items: []lore.Item{}}
	if collection.IndexGuide != nil {
		native.IndexGuide = *collection.IndexGuide
	}
	if len(native.Categories) == 0 {
		native.Version = 2 // Released collections without definitions use the legacy mapping.
	}
	ids := []string{}
	for _, raw := range collection.Items {
		if err := validatePayload("lore.entry", raw); err != nil {
			return collection, nil, err
		}
		var item lore.Item
		if err := json.Unmarshal(raw, &item); err != nil {
			return collection, nil, err
		}
		if item.ID == "" || item.ID != strings.TrimSpace(item.ID) {
			return collection, nil, fmt.Errorf("invalid Lore item ID %q", item.ID)
		}
		// Portable media is validated and adopted separately after the native entry.
		item.Materials = nil
		ids = append(ids, item.ID)
		native.Items = append(native.Items, item)
	}
	data, err := json.Marshal(native)
	if err != nil {
		return collection, nil, err
	}
	validated, err := lore.DecodeCollection(data)
	if err != nil {
		return collection, nil, err
	}
	for i, item := range validated.Items {
		if item.ID != ids[i] {
			return collection, nil, fmt.Errorf("Lore item ID must be canonical: %q", ids[i])
		}
	}
	collection.Categories = validated.Categories
	return collection, validated.Items, nil
}

func loreDigest(item lore.Item) string {
	// Timestamps are bookkeeping, not user edits. Resolved materials include the
	// asset identity and per-item association text, so both participate in checks.
	item.CreatedAt, item.UpdatedAt = "", ""
	raw, _ := json.Marshal(item)
	return revisionfile.Revision(raw)
}

func loreMembersState(items []lore.Item, binding Binding) string {
	digests := map[string]string{}
	for _, item := range items {
		digests[item.ID] = loreDigest(item)
	}
	return collectionMembersState(digests, binding)
}

func (s *Service) exportLoreCollection(ctx context.Context, ref LocalRef) (map[string][]byte, error) {
	_, layout, err := s.registry.Resolve(ref.ProjectID, true)
	if err != nil {
		return nil, err
	}
	items, err := lore.NewStore(layout.ContentRoot).ListAll()
	if err != nil {
		return nil, err
	}
	categories, err := lore.NewStore(layout.ContentRoot).Categories()
	if err != nil {
		return nil, err
	}
	guide, err := lore.NewStore(layout.ContentRoot).IndexGuide()
	if err != nil {
		return nil, err
	}
	sourceIDs, err := s.collectionSourceIDs(ctx, ref)
	if err != nil {
		return nil, err
	}
	collection := portableCollection[json.RawMessage]{Version: 1, Items: []json.RawMessage{}, IndexGuide: &guide.Guide}
	files := map[string][]byte{}
	usedCategories := map[string]bool{"character": true}
	exportedIDs := map[string]string{}
	for _, item := range items {
		sourceID := item.ID
		if ref.ID != "all" {
			var found bool
			sourceID, found = sourceIDs[item.ID]
			if !found {
				continue
			}
		}
		usedCategories[item.Type] = true
		exportedIDs[item.ID] = sourceID
		item.ID = sourceID
		raw, err := portableJSON("lore.entry", item)
		if err != nil {
			return nil, err
		}
		payload, err := s.exportLoreMaterials(ctx, ref, item, raw)
		if err != nil {
			return nil, err
		}
		collection.Items = append(collection.Items, payload["resource.json"])
		delete(payload, "resource.json")
		maps.Copy(files, payload)
	}
	if len(collection.Items) == 0 {
		return nil, fmt.Errorf("Lore collection is empty")
	}
	collection.IndexGuide.ItemOrder = remapLoreIndexItemOrder(collection.IndexGuide.ItemOrder, exportedIDs)
	for _, category := range categories {
		if usedCategories[category.ID] {
			collection.Categories = append(collection.Categories, category)
		}
	}
	raw, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return nil, err
	}
	files["resource.json"] = raw
	return files, nil
}

func (s *Service) stageLoreCollection(ctx context.Context, previewDir string, resource PreviewResource, binding *Binding, raw []byte, review *updateReview, staged map[FileTarget][]byte, extra *[]FileTarget, importedAssets map[FileTarget]lore.Asset) error {
	portable, incoming, err := readLoreCollection(raw)
	if err != nil {
		return err
	}
	target := FileTarget{ProjectID: binding.Local.ProjectID, Path: lore.ItemsRelativePath}
	dir, err := os.MkdirTemp("", "denova-lore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if len(staged[target]) > 0 {
		if err := writeFiles(dir, map[string][]byte{target.Path: staged[target]}); err != nil {
			return err
		}
	}
	store := lore.NewStore(dir)
	current, err := store.ListAll()
	if err != nil {
		return err
	}
	var collection lore.Collection
	if len(staged[target]) > 0 {
		collection, err = lore.DecodeCollection(staged[target])
		if err != nil {
			return err
		}
	} else {
		collection.Version = 3
		collection.Categories = lore.DefaultCategories()
	}
	for _, category := range portable.Categories {
		if lore.HasCategory(collection.Categories, category.ID) {
			continue
		}
		// Existing names belong to the user. Keep incoming identity and choose
		// a distinct display name without changing local category definitions.
		base := category.DisplayName()
		for suffix := 2; lore.CategoryNameConflict(collection.Categories, category); suffix++ {
			category.Name = fmt.Sprintf("%s (%d)", base, suffix)
		}
		collection.Categories = append(collection.Categories, category)
	}
	if portable.IndexGuide != nil {
		if collection.IndexGuide.AutomaticDetails == nil {
			collection.IndexGuide.AutomaticDetails = map[string]string{}
		}
		for key, detail := range portable.IndexGuide.AutomaticDetails {
			if _, exists := collection.IndexGuide.AutomaticDetails[key]; !exists {
				collection.IndexGuide.AutomaticDetails[key] = detail
			}
		}
		// Project-owned sections win by stable identity. New imported sections
		// keep their presets; names are disambiguated for exact tool lookup.
		if collection.IndexGuide.IntroMarkdown == "" {
			collection.IndexGuide.IntroMarkdown = portable.IndexGuide.IntroMarkdown
		}
		for _, group := range portable.IndexGuide.Groups {
			if slices.ContainsFunc(collection.IndexGuide.Groups, func(g lore.IndexGroup) bool { return g.ID == group.ID }) {
				continue
			}
			base := group.Name
			for suffix := 2; slices.ContainsFunc(collection.IndexGuide.Groups, func(g lore.IndexGroup) bool { return strings.EqualFold(g.Name, group.Name) }); suffix++ {
				group.Name = fmt.Sprintf("%s (%d)", base, suffix)
			}
			collection.IndexGuide.Groups = append(collection.IndexGuide.Groups, group)
		}
		for _, key := range portable.IndexGuide.GroupOrder {
			if !slices.Contains(collection.IndexGuide.GroupOrder, key) {
				collection.IndexGuide.GroupOrder = append(collection.IndexGuide.GroupOrder, key)
			}
		}
	}
	if binding.Members == nil {
		binding.Members = map[string]CollectionMember{}
	}
	reserved := slices.Clone(collection.Items)
	// A missing local item still owns its receipt identity for future updates.
	for _, member := range binding.Members {
		reserved = append(reserved, lore.Item{ID: member.ID})
	}
	// Snapshot existing names before the merge loop so detail-aware duplicate
	// resolution never matches an incoming item appended earlier in the loop.
	existingByName := make(map[string]lore.Item, len(collection.Items))
	for _, item := range collection.Items {
		if key := lore.NameKey(item.Name); key != "" {
			existingByName[key] = item
		}
	}
	applied := map[string]bool{}
	// Merge by receipt identity, never by name. Missing upstream items remain local.
	for i := range incoming {
		sourceID := incoming[i].ID
		member, found := binding.Members[sourceID]
		if !found {
			if existing, ok := existingByName[lore.NameKey(incoming[i].Name)]; ok && review.overrideByDetail {
				if !moreDetailed(incoming[i], existing) {
					review.skipped = append(review.skipped, incoming[i].Name)
					continue
				}
				review.overrides = append(review.overrides, incoming[i].Name)
				member.ID = existing.ID
			} else {
				member.ID, err = lore.NewItemID(reserved, incoming[i].Name)
				if err != nil {
					return err
				}
				reserved = append(reserved, lore.Item{ID: member.ID})
			}
		}
		var payload struct {
			Materials *portableMaterials `json:"materials"`
		}
		if err := json.Unmarshal(portable.Items[i], &payload); err != nil {
			return err
		}
		materials := portableMaterials{}
		if payload.Materials != nil {
			materials = *payload.Materials
		}
		sourceDigest, err := loreUpdateDigest(incoming[i], materials, func(name string) ([]byte, error) { return resourceAsset(previewDir, resource, name) })
		if err != nil {
			return err
		}
		currentDigest, localComparable := "missing", "missing"
		var currentItem lore.Item
		localBaselines := map[string]string{}
		for _, item := range current {
			if item.ID != member.ID {
				continue
			}
			currentItem = item
			currentDigest = loreDigest(item)
			for _, material := range item.ResolvedMaterials {
				if material.Path == "" {
					continue
				}
				snapshot, readErr := s.snapshot(ctx, FileTarget{ProjectID: binding.Local.ProjectID, Path: material.Path})
				if readErr != nil {
					return readErr
				}
				localBaselines[material.Path] = snapshot.Revision
				if baseline, tracked := binding.Baseline[material.Path]; tracked && snapshot.Revision != baseline {
					currentDigest = "modified-material:" + snapshot.Revision
				}
			}
			localComparable, err = s.localLoreUpdateDigest(ctx, binding.Local.ProjectID, item)
			if err != nil {
				if !os.IsNotExist(err) {
					return err
				}
				localComparable = "missing-material"
			}
		}
		// Local digests retain identity to detect deletions/edits; semantic
		// equality with upstream additionally ignores generated material IDs.
		incomingDigest := sourceDigest
		if localComparable == sourceDigest {
			incomingDigest = currentDigest
		}
		apply, acknowledge := review.decide(binding.ResourceID, sourceID, incoming[i].Name, member.SourceDigest, sourceDigest, member.Digest, currentDigest, incomingDigest)
		if acknowledge {
			member.SourceDigest = sourceDigest
		}
		member.UpstreamRemoved = false
		if !apply {
			if localComparable == sourceDigest {
				member.Digest = loreDigest(currentItem)
				maps.Copy(binding.Baseline, localBaselines)
				// Reuse stable material identities when only package recommendations
				// change. No content or Project settings are rewritten by adoption.
				if payload.Materials != nil {
					for i, material := range payload.Materials.Entries {
						if material.AssetPath != "" && i < len(currentItem.ResolvedMaterials) {
							importedAssets[FileTarget{ProjectID: binding.Local.ProjectID, Path: path.Join(resource.Root, material.AssetPath)}] = currentItem.ResolvedMaterials[i].Asset
						}
					}
				}
			}
			binding.Members[sourceID] = member
			continue
		}
		applied[sourceID] = true
		incoming[i].ID = member.ID
		at := slices.IndexFunc(collection.Items, func(item lore.Item) bool { return item.ID == member.ID })
		if at >= 0 {
			previous := collection.Items[at]
			incoming[i].CreatedAt = previous.CreatedAt
			if portable.IndexGuide == nil {
				incoming[i].IndexMemberships = previous.IndexMemberships
			}
			incoming[i].Materials, incoming[i].Image = previous.Materials, previous.Image
			collection.Items[at] = incoming[i]
		} else {
			collection.Items = append(collection.Items, incoming[i])
		}
		binding.Members[sourceID] = member
	}
	if portable.IndexGuide != nil {
		localIDs := map[string]string{}
		for sourceID, member := range binding.Members {
			localIDs[sourceID] = member.ID
		}
		if collection.IndexGuide.ItemOrder == nil {
			collection.IndexGuide.ItemOrder = map[string][]string{}
		}
		// Local order wins. Remap package IDs before appending new positions.
		for key, order := range remapLoreIndexItemOrder(portable.IndexGuide.ItemOrder, localIDs) {
			for _, id := range order {
				if !slices.Contains(collection.IndexGuide.ItemOrder[key], id) {
					collection.IndexGuide.ItemOrder[key] = append(collection.IndexGuide.ItemOrder[key], id)
				}
			}
		}
	}
	content, err := json.Marshal(collection)
	if err != nil {
		return err
	}
	if err := writeFiles(dir, map[string][]byte{target.Path: content}); err != nil {
		return err
	}
	// Reuse the native collection validator for collisions with existing items.
	if _, err := store.ListAll(); err != nil {
		return err
	}
	for i, item := range incoming {
		if !applied[collectionSourceID(portable.Items[i])] {
			continue
		}
		var payload struct {
			Materials *portableMaterials `json:"materials"`
		}
		if err := json.Unmarshal(portable.Items[i], &payload); err != nil {
			return err
		}
		if item.Materials == nil && item.Image == nil && (payload.Materials == nil || payload.Materials.Entries != nil && len(payload.Materials.Entries) == 0 && payload.Materials.CoverPath == "" && payload.Materials.CoverURL == "") {
			continue
		}
		local := binding.Local
		local.ID = item.ID
		if err := importLoreMaterials(ctx, dir, previewDir, resource, local, portable.Items[i], staged, extra, importedAssets); err != nil {
			return err
		}
	}
	items, err := store.ListAll()
	if err != nil {
		return err
	}
	byID := map[string]lore.Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for i, raw := range portable.Items {
		var identity struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &identity)
		if !applied[identity.ID] {
			continue
		}
		member := binding.Members[identity.ID]
		member.Digest = loreDigest(byID[incoming[i].ID])
		binding.Members[identity.ID] = member
	}
	retainedPaths := map[string]bool{}
	for _, member := range binding.Members {
		for _, material := range byID[member.ID].ResolvedMaterials {
			if material.Path != "" {
				retainedPaths[material.Path] = true
			}
		}
	}
	for name := range binding.Baseline {
		if !retainedPaths[name] {
			delete(binding.Baseline, name)
		}
	}
	for id, member := range binding.Members {
		if !slices.ContainsFunc(portable.Items, func(raw json.RawMessage) bool { return collectionSourceID(raw) == id }) {
			member.UpstreamRemoved = true
			binding.Members[id] = member
			review.items = append(review.items, UpdateItem{ResourceID: binding.ResourceID, MemberID: id, Name: id, State: "upstream_removed"})
		}
	}
	staged[target], err = os.ReadFile(filepath.Join(dir, filepath.FromSlash(target.Path)))
	return err
}

// Collection receipts translate item identity at the package boundary. Sorting
// is presentation metadata, so references outside the exported set are omitted.
func remapLoreIndexItemOrder(orders map[string][]string, ids map[string]string) map[string][]string {
	result := map[string][]string{}
	for key, order := range orders {
		for _, id := range order {
			if mapped := ids[id]; mapped != "" {
				result[key] = append(result[key], mapped)
			}
		}
	}
	return result
}

// detailFields reports the detail measure of a lore item: the non-empty field
// count recorded by the importer, falling back to 1 for any item with content
// so hand-written or legacy entries still compare sensibly.
func detailFields(item lore.Item) int {
	if item.DetailFields > 0 {
		return item.DetailFields
	}
	if strings.TrimSpace(item.Content) != "" {
		return 1
	}
	return 0
}

// moreDetailed reports whether a carries a richer setting than b: more populated
// detail fields win first, then a longer body. Equality is not "more detailed".
func moreDetailed(a, b lore.Item) bool {
	af, bf := detailFields(a), detailFields(b)
	if af != bf {
		return af > bf
	}
	return utf8.RuneCountInString(a.Content) > utf8.RuneCountInString(b.Content)
}
