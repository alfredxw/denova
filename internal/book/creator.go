package book

import (
	"fmt"
	"os"
	"path/filepath"

	"denova/internal/prompts"
)

// CreatorFileName 创作者指令文件名，存于 workspace 根目录。
const CreatorFileName = "CREATOR.md"

// ensureCreatorTemplate 在 workspace 根目录写入 CREATOR.md 模板（仅当文件不存在时）。
func ensureCreatorTemplate(workspace string) error {
	path := filepath.Join(workspace, CreatorFileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查 %s 失败: %w", CreatorFileName, err)
	}
	if err := os.WriteFile(path, []byte(prompts.CreatorTemplate), 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", CreatorFileName, err)
	}
	return nil
}
