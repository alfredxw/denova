package resourceexchange

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"denova/internal/revisionfile"
	"github.com/google/uuid"
)

type Backup struct {
	ID        string    `json:"backup_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
	Files     int       `json:"files"`
}

func (s *Service) readBackup(ctx context.Context, id string) (transaction, error) {
	if _, err := uuid.Parse(id); err != nil {
		return transaction{}, err
	}
	// Migration backups stay administrative records, never reintroduce a second
	// source authority through the product's install rollback action.
	if _, err := s.ReadPlan(ctx, id); err != nil {
		return transaction{}, err
	}
	snapshot, err := revisionfile.Read(ctx, filepath.Join(s.root, "resource-exchange", "transactions", id+".json"))
	if err != nil {
		return transaction{}, err
	}
	var txn transaction
	err = json.Unmarshal(snapshot.Content, &txn)
	return txn, err
}

func (s *Service) Backups(ctx context.Context, installationID string) ([]Backup, error) {
	if _, err := uuid.Parse(installationID); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(s.root, "resource-exchange", "transactions"))
	if os.IsNotExist(err) {
		return []Backup{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Backup{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		txn, err := s.readBackup(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			continue
		}
		if !slices.ContainsFunc(txn.Changes, func(change fileChange) bool { return change.Target == installationTarget(installationID) }) {
			continue
		}
		result = append(result, Backup{ID: txn.ID, State: txn.State, CreatedAt: txn.CreatedAt, Files: len(txn.Changes)})
	}
	slices.SortFunc(result, func(a, b Backup) int { return b.CreatedAt.Compare(a.CreatedAt) })
	return result, nil
}

func (s *Service) DownloadBackup(ctx context.Context, id string) ([]byte, error) {
	txn, err := s.readBackup(ctx, id)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, change := range txn.Changes {
		if !change.Existed {
			continue
		}
		prefix := "global"
		if change.Target.ProjectID != "" {
			prefix = "projects/" + change.Target.ProjectID
		}
		files[prefix+"/"+change.Target.Path] = change.Before
	}
	metadata, err := json.MarshalIndent(Backup{ID: txn.ID, State: txn.State, CreatedAt: txn.CreatedAt, Files: len(txn.Changes)}, "", "  ")
	if err != nil {
		return nil, err
	}
	files["backup.json"] = metadata
	return archiveBytes(files)
}

// PlanRestore restores a committed installation only while every affected target
// still matches that commit. Unknown later edits are never overwritten.
func (s *Service) PlanRestore(ctx context.Context, id string) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	txn, err := s.readBackup(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	if txn.State != "committed" {
		return Plan{}, fmt.Errorf("backup is not a committed installation")
	}
	original, err := s.ReadPlan(ctx, id)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{ID: uuid.NewString(), BackupID: id, ExpiresAt: time.Now().UTC().Add(time.Hour), Installation: original.Installation, Items: []PlanItem{}}
	for _, item := range original.Items {
		item.Action = "restore"
		plan.Items = append(plan.Items, item)
	}
	for _, change := range txn.Changes {
		expected := revisionfile.Revision(change.After)
		if change.Delete {
			expected = revisionfile.MissingRevision
		}
		current, err := s.snapshot(ctx, change.Target)
		if err != nil {
			return Plan{}, err
		}
		if current.Revision != expected {
			return Plan{}, ErrLocalModified
		}
		plan.Changes = append(plan.Changes, fileChange{Target: change.Target, Expected: expected, After: change.Before, Delete: !change.Existed})
	}
	if original.PlatformState != "" {
		plan.PlatformState, err = s.platform.InstallState()
		if err != nil {
			return Plan{}, err
		}
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return Plan{}, err
	}
	_, err = revisionfile.ReplaceIfRevision(ctx, filepath.Join(s.root, "resource-exchange", "plans", plan.ID+".json"), revisionfile.MissingRevision, raw, revisionfile.Options{})
	return plan, err
}
