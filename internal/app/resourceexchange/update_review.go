package resourceexchange

import (
	"encoding/json"
	"fmt"
	"slices"

	"denova/internal/revisionfile"
)

// UpdateItem is a review decision for one independently replaceable content unit.
// Unresolved conflicts preserve local content and remain eligible for later review.
type UpdateItem struct {
	ResourceID string `json:"resource_id"`
	MemberID   string `json:"member_id,omitempty"`
	Name       string `json:"name"`
	State      string `json:"state"`
	Conflict   bool   `json:"conflict,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Missing    bool   `json:"missing,omitempty"`
}

type updateReview struct {
	choices          map[string]map[string]string
	items            []UpdateItem
	overrideByDetail bool
	overrides        []string
	skipped          []string
}

func (r *updateReview) decide(resource, member, name, sourceBase, sourceNext, localBase, localNow, localNext string) (apply, acknowledge bool) {
	item := UpdateItem{ResourceID: resource, MemberID: member, Name: name}
	acknowledge = true
	switch {
	case localBase == "":
		item.State, apply = "create", true
	case localNow == localNext:
		item.State = "unchanged"
	case sourceBase != "" && sourceBase == sourceNext:
		item.State = "keep"
		if localNow == localBase {
			item.State = "unchanged"
		}
	case localNow == localBase:
		item.State, apply = "update", true
	default:
		item.Conflict = true
		item.Resolution = r.choices[resource][member]
		switch item.Resolution {
		case "keep":
			item.State = "keep"
		case "remote":
			item.State, apply = "update", true
		default:
			item.State, acknowledge = "conflict", false
		}
	}
	item.Missing = !apply && localNow == "missing"
	r.items = append(r.items, item)
	return
}

func (r *updateReview) pending(resource string) bool {
	return slices.ContainsFunc(r.items, func(item UpdateItem) bool {
		return item.ResourceID == resource && (item.State == "conflict" || item.State == "blocked" || item.Missing)
	})
}

func (r *updateReview) validate() error {
	for resource, members := range r.choices {
		for member, choice := range members {
			if choice != "" && choice != "keep" && choice != "remote" {
				return fmt.Errorf("invalid update resolution for %s/%s", resource, member)
			}
		}
	}
	return nil
}

// Compare package metadata and the complete upstream resource set, not versions
// or Git commits alone. Reuse existing resource digests for actual content.
func reviewedSource(candidate PackagePreview) string {
	type entry struct {
		Resource
		Digest string
	}
	resources := make([]entry, 0, len(candidate.Resources))
	for _, resource := range candidate.Resources {
		resources = append(resources, entry{resource.Resource, resource.Digest})
	}
	slices.SortFunc(resources, func(a, b entry) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	raw, _ := json.Marshal(struct {
		Package   PackageInfo
		Resources []entry
		Defaults  *PackageGameDefaults
	}{candidate.Package, resources, candidate.GameDefaults})
	return revisionfile.Revision(raw)
}
