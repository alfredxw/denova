package interactive

import (
	"fmt"
	"log"
	"time"
)

// MigrateStoryMemoryStructuresToDirectorModules moves legacy story-local
// structure definitions into reusable director modules. Runtime records stay
// in the story memory book and are not rewritten.
func (s *Store) MigrateStoryMemoryStructuresToDirectorModules() error {
	if s == nil || s.novaDir == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := NewStoryMemoryStructureLibrary(s.novaDir).ensureBuiltins(); err != nil {
		return err
	}
	index, err := s.readIndexLocked()
	if err != nil {
		return err
	}
	if len(index.Stories) == 0 {
		return nil
	}
	directorUsage := map[string]int{}
	for _, story := range index.Stories {
		directorID := NormalizeStoryDirectorID(story.StoryDirectorID)
		if directorID == "" {
			directorID = DefaultStoryDirectorID
		}
		directorUsage[directorID]++
	}

	defaultStructures := DefaultStoryMemoryStructureModule().Structures
	memoryLibrary := NewStoryMemoryStructureLibrary(s.novaDir)
	directorLibrary := NewStoryDirectorLibrary(s.novaDir)
	indexChanged := false
	for _, story := range index.Stories {
		meta, lines, err := s.readStoryLocked(story.ID)
		if err != nil {
			return err
		}
		book, err := s.readMemoryBookLocked(story.ID)
		if err != nil {
			return err
		}
		if storyMemoryStructuresEqual(book.Structures, defaultStructures) {
			continue
		}
		directorID := NormalizeStoryDirectorID(meta.StoryDirectorID)
		if directorID == "" {
			directorID = DefaultStoryDirectorID
		}
		director, err := directorLibrary.Get(directorID)
		if err != nil {
			return err
		}
		refs := NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
		if refs.MemoryStructureDisabled || (refs.MemoryStructureID != "" && refs.MemoryStructureID != DefaultStoryMemoryStructureModuleID) {
			continue
		}

		moduleID := normalizeDirectorModuleID(fmt.Sprintf("story-%s-memory", story.ID))
		module := StoryMemoryStructureModule{
			ID:          moduleID,
			Name:        trimBytes(firstNonEmptyString(story.Title, meta.Title, story.ID)+" 记忆结构", 256),
			Description: "由旧故事级记忆结构迁移生成。运行时记录仍保留在原故事记忆文件中。",
			Structures:  book.Structures,
		}
		if _, err := memoryLibrary.Get(moduleID); err == nil {
			if _, err := memoryLibrary.Update(moduleID, module, ""); err != nil {
				return err
			}
		} else {
			if _, err := memoryLibrary.Create(module); err != nil {
				return err
			}
		}

		targetDirectorID := directorID
		if IsBuiltinStoryDirectorID(directorID) || directorUsage[directorID] > 1 {
			targetDirectorID = normalizeDirectorModuleID(fmt.Sprintf("story-%s-director", story.ID))
			director.ID = targetDirectorID
			director.Name = trimBytes(firstNonEmptyString(story.Title, meta.Title, story.ID)+" 故事导演", 256)
			director.Description = trimBytes("由旧故事级记忆结构迁移生成的故事专属导演。", 1024)
		}
		director.ModuleRefs = NormalizeStoryDirectorModuleRefs(director.ModuleRefs)
		director.ModuleRefs.MemoryStructureID = moduleID
		director.ModuleRefs.MemoryStructureDisabled = false
		director.Path = ""
		director.Custom = false
		director.BuiltinOverridden = false
		director.Invalid = false
		director.Error = ""
		if targetDirectorID == directorID {
			if _, err := directorLibrary.Update(directorID, director, ""); err != nil {
				return err
			}
		} else if _, err := directorLibrary.Get(targetDirectorID); err == nil {
			if _, err := directorLibrary.Update(targetDirectorID, director, ""); err != nil {
				return err
			}
		} else {
			if _, err := directorLibrary.Create(director); err != nil {
				return err
			}
		}

		if meta.StoryDirectorID != targetDirectorID {
			now := time.Now().UTC().Format(time.RFC3339Nano)
			meta.StoryDirectorID = targetDirectorID
			meta.UpdatedAt = now
			if err := s.rewriteStoryLocked(story.ID, meta, lines); err != nil {
				return err
			}
			for i := range index.Stories {
				if index.Stories[i].ID == story.ID {
					index.Stories[i].StoryDirectorID = targetDirectorID
					index.Stories[i].UpdatedAt = now
					indexChanged = true
					break
				}
			}
		}
		log.Printf("[interactive-memory] migrated story memory structures story_id=%s module_id=%s director_id=%s", story.ID, moduleID, targetDirectorID)
	}
	if indexChanged {
		return s.writeIndexLocked(index)
	}
	return nil
}
