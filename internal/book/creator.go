package book

import (
	"fmt"
	"os"
	"path/filepath"
)

// ensureCreatorTemplate 在 workspace 根目录写入 CREATOR.md 模板（仅当文件不存在时）。
func ensureCreatorTemplate(workspace string) error {
	path := filepath.Join(workspace, CreatorFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查 %s 失败: %w", CreatorFileName, err)
	}
	if err := os.WriteFile(path, []byte(CreatorTemplate), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", CreatorFileName, err)
	}
	return nil
}
