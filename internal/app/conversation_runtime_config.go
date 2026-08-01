package app

import (
	"fmt"
	"log"

	"denova/config"
	"denova/internal/agents/session"
	"denova/internal/conversationconfig"
	"denova/internal/interactive"
)

func recentConversationSeed(store *session.Store, runtimeCfg *config.Config, agentKind, excludeID string) (conversationconfig.Config, error) {
	if store != nil {
		recent, ok, err := store.RecentRuntimeConfig(agentKind, excludeID)
		if err != nil {
			return conversationconfig.Config{}, err
		}
		if ok {
			if err := conversationconfig.Validate(runtimeCfg, recent, agentKind); err == nil {
				return recent, nil
			} else {
				log.Printf("[conversation-config] recent selection is no longer usable agent_kind=%s profile_id=%s error=%v; using Settings default", agentKind, recent.ProfileID, err)
			}
		}
	}
	seed := conversationconfig.Default(runtimeCfg, agentKind)
	if err := conversationconfig.Validate(runtimeCfg, seed, agentKind); err != nil {
		return conversationconfig.Config{}, fmt.Errorf("resolve default conversation config: %w", err)
	}
	return seed, nil
}

func recentInteractiveConversationSeed(store *interactive.Store, runtimeCfg *config.Config, excludeStoryID string) (conversationconfig.Config, error) {
	if store != nil {
		recent, ok, err := store.RecentRuntimeConfig(excludeStoryID)
		if err != nil {
			return conversationconfig.Config{}, err
		}
		if ok {
			if err := conversationconfig.Validate(runtimeCfg, recent, config.AgentKindInteractiveStory); err == nil {
				return recent, nil
			} else {
				log.Printf("[conversation-config] recent Game selection is no longer usable profile_id=%s error=%v; using Settings default", recent.ProfileID, err)
			}
		}
	}
	seed := conversationconfig.Default(runtimeCfg, config.AgentKindInteractiveStory)
	if err := conversationconfig.Validate(runtimeCfg, seed, config.AgentKindInteractiveStory); err != nil {
		return conversationconfig.Config{}, err
	}
	return seed, nil
}

func ensureExistingSessionConfig(sess *session.Session, runtimeCfg *config.Config, agentKind string) (conversationconfig.Snapshot, error) {
	if sess == nil {
		return conversationconfig.Snapshot{}, fmt.Errorf("conversation session is nil")
	}
	if snapshot, ok := sess.RuntimeConfig(); ok {
		if snapshot.AgentKind != agentKind {
			return conversationconfig.Snapshot{}, fmt.Errorf("conversation Agent kind is %q, expected %q", snapshot.AgentKind, agentKind)
		}
		return snapshot, nil
	}
	seed := conversationconfig.Default(runtimeCfg, agentKind)
	if err := conversationconfig.Validate(runtimeCfg, seed, agentKind); err != nil {
		return conversationconfig.Snapshot{}, err
	}
	return sess.EnsureRuntimeConfig(seed)
}

func getOrCreateConversationSession(store *session.Store, sessionID string, runtimeCfg *config.Config, agentKind string) (*session.Session, conversationconfig.Snapshot, error) {
	return conversationSessionConfig(store, sessionID, runtimeCfg, agentKind, true)
}

// previewConversationSessionConfig resolves the exact snapshot a new
// conversation would inherit without turning an ephemeral UI draft into a
// durable empty session.
func previewConversationSessionConfig(store *session.Store, sessionID string, runtimeCfg *config.Config, agentKind string) (conversationconfig.Snapshot, error) {
	_, snapshot, err := conversationSessionConfig(store, sessionID, runtimeCfg, agentKind, false)
	return snapshot, err
}

func conversationSessionConfig(store *session.Store, sessionID string, runtimeCfg *config.Config, agentKind string, create bool) (*session.Session, conversationconfig.Snapshot, error) {
	if store == nil {
		return nil, conversationconfig.Snapshot{}, ErrNoWorkspace
	}
	if store.Exists(sessionID) {
		sess, err := store.Get(sessionID)
		if err != nil {
			return nil, conversationconfig.Snapshot{}, err
		}
		snapshot, err := ensureExistingSessionConfig(sess, runtimeCfg, agentKind)
		return sess, snapshot, err
	}
	seed, err := recentConversationSeed(store, runtimeCfg, agentKind, sessionID)
	if err != nil {
		return nil, conversationconfig.Snapshot{}, err
	}
	if !create {
		return nil, conversationconfig.Snapshot{Config: seed}, nil
	}
	sess, err := store.GetOrCreateWithRuntimeConfig(sessionID, seed)
	if err != nil {
		return nil, conversationconfig.Snapshot{}, err
	}
	snapshot, ok := sess.RuntimeConfig()
	if !ok {
		return nil, conversationconfig.Snapshot{}, conversationconfig.ErrNotInitialized
	}
	return sess, snapshot, nil
}

func applySessionConversationConfig(sess *session.Session, runtimeCfg *config.Config, agentKind string) (conversationconfig.Snapshot, error) {
	snapshot, err := ensureExistingSessionConfig(sess, runtimeCfg, agentKind)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if err := conversationconfig.Apply(runtimeCfg, snapshot.Config); err != nil {
		return conversationconfig.Snapshot{}, fmt.Errorf("apply conversation runtime config: %w", err)
	}
	return snapshot, nil
}

func applyInteractiveConversationConfig(store *interactive.Store, runtimeCfg *config.Config, storyID, branchID string) (conversationconfig.Snapshot, error) {
	if store == nil {
		return conversationconfig.Snapshot{}, ErrNoWorkspace
	}
	snapshot, ok, err := store.BranchRuntimeConfig(storyID, branchID)
	if err != nil {
		return conversationconfig.Snapshot{}, err
	}
	if !ok {
		seed := conversationconfig.Default(runtimeCfg, config.AgentKindInteractiveStory)
		if err := conversationconfig.Validate(runtimeCfg, seed, config.AgentKindInteractiveStory); err != nil {
			return conversationconfig.Snapshot{}, err
		}
		snapshot, err = store.EnsureBranchRuntimeConfig(storyID, branchID, seed)
		if err != nil {
			return conversationconfig.Snapshot{}, err
		}
	}
	if err := conversationconfig.Apply(runtimeCfg, snapshot.Config); err != nil {
		return conversationconfig.Snapshot{}, fmt.Errorf("apply interactive conversation runtime config: %w", err)
	}
	return snapshot, nil
}
