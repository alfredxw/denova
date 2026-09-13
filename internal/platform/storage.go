package platform

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"denova/internal/portablepath"
	"github.com/google/uuid"
)

// An extension owns its installation, releases, settings and private data.
// Project stores and canonical Agent journals keep their existing ownership.
func (m *Manager) packagePath(ref PackageRef) string {
	return filepath.Join(m.root, ref.Kind.directory(), ref.ID)
}

func (m *Manager) saveInstalled(kind Kind, item Installed) error {
	return writeJSON(filepath.Join(m.packagePath(PackageRef{Kind: kind, ID: item.ID}), "installed.json"), item)
}

func (m *Manager) packageIDs(kind Kind) ([]string, error) {
	if !kind.valid() {
		return nil, failure("INVALID_ARGUMENT", "Unknown package kind %q", kind)
	}
	root := filepath.Join(m.root, kind.directory())
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := validateID(entry.Name()); err != nil {
			return nil, err
		}
		if err := portablepath.CheckNoCollision(root, entry.Name()); err != nil {
			return nil, err
		}
		ids = append(ids, entry.Name())
	}
	return ids, nil
}

func (m *Manager) List(kind Kind) ([]Installed, error) {
	ids, err := m.packageIDs(kind)
	if err != nil {
		return nil, err
	}
	items := []Installed{}
	for _, id := range ids {
		var item Installed
		err := readJSON(filepath.Join(m.packagePath(PackageRef{Kind: kind, ID: id}), "installed.json"), &item)
		if os.IsNotExist(err) {
			continue
		} // A preview need not be installed.
		if err != nil {
			return nil, err
		}
		if item.ID != id {
			return nil, failure("INVALID_PACKAGE", "Installation identity does not match directory %s", id)
		}
		items = append(items, item)
	}
	return items, nil
}

func (m *Manager) instancePath(gameID, id string) string {
	return filepath.Join(m.packagePath(PackageRef{Kind: Game, ID: gameID}), "instances", id)
}

// Instance IDs remain the public identity. Directory discovery avoids a second
// persisted index that could disagree with the instance's canonical metadata.
func (m *Manager) Instance(id string) (Instance, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Instance{}, failure("INVALID_ARGUMENT", "Invalid instance ID")
	}
	ids, err := m.packageIDs(Game)
	if err != nil {
		return Instance{}, err
	}
	for _, gameID := range ids {
		var item Instance
		err := readJSON(filepath.Join(m.instancePath(gameID, id), "instance.json"), &item)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return Instance{}, err
		}
		if item.ID != id || item.GameID != gameID {
			return Instance{}, failure("INVALID_PACKAGE", "Instance identity does not match directory")
		}
		return item, nil
	}
	return Instance{}, failure("NOT_FOUND", "Instance %s is unavailable", id)
}

func (m *Manager) Instances() ([]Instance, error) {
	ids, err := m.packageIDs(Game)
	if err != nil {
		return nil, err
	}
	items := []Instance{}
	for _, gameID := range ids {
		entries, err := os.ReadDir(filepath.Join(m.packagePath(PackageRef{Kind: Game, ID: gameID}), "instances"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if _, err := uuid.Parse(entry.Name()); err != nil {
				return nil, err
			}
			var item Instance
			if err := readJSON(filepath.Join(m.instancePath(gameID, entry.Name()), "instance.json"), &item); err != nil {
				return nil, err
			}
			if item.ID != entry.Name() || item.GameID != gameID {
				return nil, failure("INVALID_PACKAGE", "Instance identity does not match directory")
			}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, nil
}
