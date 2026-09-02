// Package canonicalstore routes Denova Agent Sessions to their owning product
// journals. Root writing/chat/image/automation Sessions share the Product
// Session JSONL, game branches share the Story JSONL, and delegated children
// keep one self-contained journal below the same Project Store.
package canonicalstore

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	agentrun "denova/internal/agents/run"
	productsession "denova/internal/agents/session"
	"denova/internal/interactive"
	"denova/internal/project"

	agent "github.com/alfredxw/denova/agent"
	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

type Store struct {
	dataDir  string
	registry *project.Registry

	mu     sync.Mutex
	opened map[string]agentsession.Key
}

func New(dataDir string, registry *project.Registry) (*Store, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" || registry == nil {
		return nil, fmt.Errorf("canonical Agent Session Store requires data directory and Project Registry")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	return &Store{dataDir: filepath.Clean(absolute), registry: registry, opened: make(map[string]agentsession.Key)}, nil
}

func (store *Store) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	key, err := agentsession.NormalizeKey(key)
	if err != nil {
		return nil, err
	}
	root, child, err := rootSessionKey(key)
	if err != nil {
		return nil, err
	}
	binding, layout, err := store.resolve(root, true)
	if err != nil {
		return nil, err
	}
	log, err := store.openResolved(ctx, key, binding, layout, child)
	if err != nil {
		return nil, err
	}
	canonical, _ := agentsession.CanonicalKey(key)
	store.mu.Lock()
	store.opened[canonical] = cloneKey(key)
	store.mu.Unlock()
	return log, nil
}

func (store *Store) openResolved(
	ctx context.Context,
	key agentsession.Key,
	binding agentrun.RuntimeBinding,
	layout project.Layout,
	child bool,
) (agentsession.Log, error) {
	var (
		log agentsession.Log
		err error
	)
	if child {
		childStore, storeErr := sessionfile.New(filepath.Join(layout.SessionsDir(), "children"))
		if storeErr != nil {
			return nil, storeErr
		}
		log, err = childStore.Open(ctx, key)
	} else if binding.AgentKind == agentrun.AgentKindInteractiveStory {
		log, err = interactive.OpenAgentLog(ctx, layout.ContentRoot, store.dataDir, binding.StoryID, key)
	} else {
		log, err = productsession.OpenStoreAgentLog(
			ctx, layout.SessionsDir(), store.dataDir, binding.SessionID, binding.AgentKind, key,
		)
	}
	if err != nil {
		return nil, err
	}
	return log, nil
}

func (store *Store) List(ctx context.Context, selector agentsession.Selector) ([]agentsession.Key, error) {
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result := make(map[string]agentsession.Key)
	store.mu.Lock()
	for canonical, key := range store.opened {
		if selector.Matches(key) {
			result[canonical] = cloneKey(key)
		}
	}
	store.mu.Unlock()
	if err := store.discover(ctx, selector, result); err != nil {
		return nil, err
	}
	keys := make([]agentsession.Key, 0, len(result))
	for _, key := range result {
		keys = append(keys, cloneKey(key))
	}
	sort.Slice(keys, func(left, right int) bool {
		leftKey, _ := agentsession.CanonicalKey(keys[left])
		rightKey, _ := agentsession.CanonicalKey(keys[right])
		return leftKey < rightKey
	})
	return keys, nil
}

func (store *Store) Delete(ctx context.Context, key agentsession.Key) error {
	key, err := agentsession.NormalizeKey(key)
	if err != nil {
		return err
	}
	root, child, err := rootSessionKey(key)
	if err != nil {
		return err
	}
	binding, layout, err := store.resolve(root, false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, project.ErrNotFound) {
			return nil
		}
		return err
	}
	if child {
		childrenDir := filepath.Join(layout.SessionsDir(), "children")
		if _, statErr := os.Stat(childrenDir); errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		childStore, storeErr := sessionfile.New(childrenDir)
		if storeErr != nil {
			return storeErr
		}
		err = childStore.Delete(ctx, key)
	} else {
		log, openErr := store.openResolved(ctx, key, binding, layout, false)
		if errors.Is(openErr, os.ErrNotExist) {
			return nil
		}
		if openErr != nil {
			return openErr
		}
		defer log.Close()
		deletable, ok := log.(interface{ Delete(context.Context) error })
		if !ok {
			return fmt.Errorf("canonical Agent Session log cannot append a deletion tombstone")
		}
		err = deletable.Delete(ctx)
	}
	if err == nil {
		canonical, _ := agentsession.CanonicalKey(key)
		store.mu.Lock()
		delete(store.opened, canonical)
		store.mu.Unlock()
	}
	return err
}

func (store *Store) resolve(key agentsession.Key, requireAvailable bool) (agentrun.RuntimeBinding, project.Layout, error) {
	binding, err := agentrun.RuntimeBindingFromAgentSessionKey(key)
	if err != nil {
		return agentrun.RuntimeBinding{}, project.Layout{}, err
	}
	if requireAvailable {
		_, layout, resolveErr := store.registry.Resolve(binding.ProjectID, true)
		return binding, layout, resolveErr
	}
	record, err := store.registry.Get(binding.ProjectID)
	if err != nil {
		return agentrun.RuntimeBinding{}, project.Layout{}, err
	}
	layout, err := store.registry.Layout(record)
	return binding, layout, err
}

func (store *Store) discover(ctx context.Context, selector agentsession.Selector, result map[string]agentsession.Key) error {
	records, err := store.registry.List(true)
	if err != nil {
		return err
	}
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		layout, layoutErr := store.registry.Layout(record)
		if layoutErr != nil {
			continue
		}
		if err := discoverChildKeys(ctx, layout.SessionsDir(), selector, result); err != nil {
			return err
		}
		if err := discoverProductKeys(layout.SessionsDir(), selector, result); err != nil {
			return err
		}
		if err := discoverStoryKeys(layout.ContentRoot, store.dataDir, selector, result); err != nil {
			return err
		}
	}
	return nil
}

func discoverChildKeys(ctx context.Context, sessionsDir string, selector agentsession.Selector, result map[string]agentsession.Key) error {
	childrenDir := filepath.Join(sessionsDir, "children")
	if _, err := os.Stat(childrenDir); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	store, err := sessionfile.New(childrenDir)
	if err != nil {
		return err
	}
	keys, err := store.List(ctx, selector)
	if err != nil {
		return err
	}
	rememberKeys(keys, selector, result)
	return nil
}

func discoverProductKeys(sessionsDir string, selector agentsession.Selector, result map[string]agentsession.Key) error {
	paths, err := filepath.Glob(filepath.Join(sessionsDir, "*.jsonl"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		keys, readErr := productsession.AgentSessionKeys(path)
		if readErr != nil {
			return readErr
		}
		rememberKeys(keys, selector, result)
	}
	return nil
}

func discoverStoryKeys(contentRoot, dataDir string, selector agentsession.Selector, result map[string]agentsession.Key) error {
	paths, err := filepath.Glob(filepath.Join(contentRoot, "interactive", "story", "story-*.jsonl"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		name := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "story-"), ".jsonl")
		if name == "" {
			continue
		}
		keys, readErr := interactive.AgentSessionKeys(contentRoot, dataDir, name)
		if readErr != nil {
			return readErr
		}
		rememberKeys(keys, selector, result)
	}
	return nil
}

func rememberKeys(keys []agentsession.Key, selector agentsession.Selector, result map[string]agentsession.Key) {
	for _, key := range keys {
		if !selector.Matches(key) {
			continue
		}
		canonical, err := agentsession.CanonicalKey(key)
		if err == nil {
			result[canonical] = cloneKey(key)
		}
	}
}

func rootSessionKey(key agentsession.Key) (agentsession.Key, bool, error) {
	root := key
	child := false
	for depth := 0; strings.HasPrefix(root.Namespace, "task."); depth++ {
		if depth >= agentsession.MaxAttributes {
			return agentsession.Key{}, false, fmt.Errorf("delegated Agent Session ancestry exceeds limit")
		}
		parent, err := agent.ParentSessionKey(root)
		if err != nil {
			return agentsession.Key{}, false, err
		}
		root, child = parent, true
	}
	return root, child, nil
}

func cloneKey(key agentsession.Key) agentsession.Key {
	key.Attributes = maps.Clone(key.Attributes)
	return key
}

var _ agentsession.Store = (*Store)(nil)
