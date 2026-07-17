package interactive

import (
	"os"
	"path/filepath"
	"strings"
)

func writeDirectorDocumentsAtomically(dir string, docs DirectorPlanDocs) error {
	contents := map[string]string{
		directorPlanFile:        docs.Plan,
		directorAgentBriefFile:  docs.AgentBrief,
		directorLoreContextFile: docs.LoreContext,
	}
	return writeDirectorDocumentContentsAtomically(dir, contents, []string{directorPlanFile, directorAgentBriefFile, directorLoreContextFile})
}

func writeDirectorDocumentChangesAtomically(dir string, before, after DirectorPlanDocs) error {
	contents := map[string]string{}
	order := make([]string, 0, 3)
	for _, document := range []struct {
		name   string
		before string
		after  string
	}{
		{name: directorPlanFile, before: before.Plan, after: after.Plan},
		{name: directorAgentBriefFile, before: before.AgentBrief, after: after.AgentBrief},
		{name: directorLoreContextFile, before: before.LoreContext, after: after.LoreContext},
	} {
		if strings.TrimSpace(document.before) == strings.TrimSpace(document.after) {
			continue
		}
		contents[document.name] = document.after
		order = append(order, document.name)
	}
	if len(order) == 0 {
		return nil
	}
	return writeDirectorDocumentContentsAtomically(dir, contents, order)
}

func writeDirectorDocumentContentsAtomically(dir string, contents map[string]string, order []string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temps := map[string]string{}
	for _, name := range order {
		temp, err := os.CreateTemp(dir, "."+name+"-*")
		if err != nil {
			removeDirectorTempFiles(temps)
			return err
		}
		path := temp.Name()
		text := strings.TrimSpace(contents[name]) + "\n"
		if _, err = temp.WriteString(text); err == nil {
			err = temp.Sync()
		}
		closeErr := temp.Close()
		if err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(path, 0o644)
		}
		if err != nil {
			_ = os.Remove(path)
			removeDirectorTempFiles(temps)
			return err
		}
		temps[name] = path
	}

	previous := map[string][]byte{}
	existed := map[string]bool{}
	for name := range contents {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			previous[name] = data
			existed[name] = true
		} else if !os.IsNotExist(err) {
			removeDirectorTempFiles(temps)
			return err
		}
	}
	for _, name := range order {
		if err := os.Rename(temps[name], filepath.Join(dir, name)); err != nil {
			for restoreName := range contents {
				target := filepath.Join(dir, restoreName)
				if existed[restoreName] {
					_ = os.WriteFile(target, previous[restoreName], 0o644)
				} else {
					_ = os.Remove(target)
				}
			}
			removeDirectorTempFiles(temps)
			return err
		}
		delete(temps, name)
	}
	return nil
}

func removeDirectorTempFiles(paths map[string]string) {
	for _, path := range paths {
		_ = os.Remove(path)
	}
}
