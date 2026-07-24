package session

import (
	"fmt"
	"os"
	"strings"
)

func rollbackJournalTail(f *os.File, validOffset int64, incompleteTail []byte) {
	if err := f.Truncate(validOffset); err != nil {
		return
	}
	if len(incompleteTail) > 0 {
		_, _ = f.Write(incompleteTail)
	}
	_ = f.Sync()
}

// preserveIncompleteTail keeps bytes that cannot be parsed as a complete final
// record before repair truncates them. Recovery never silently destroys user
// data, while the .jsonl file remains loadable on the next start.
func preserveIncompleteTail(filePath string, tail []byte) error {
	if len(tail) == 0 {
		return nil
	}
	base := filePath + ".incomplete-" + strings.TrimPrefix(newSessionID(), "s-")
	for attempt := 0; attempt < 8; attempt++ {
		backupPath := base
		if attempt > 0 {
			backupPath = fmt.Sprintf("%s-%d", base, attempt)
		}
		backup, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("备份会话 journal 不完整尾行失败: %w", err)
		}
		writeErr := writeAndSync(backup, tail)
		closeErr := backup.Close()
		if writeErr != nil {
			return fmt.Errorf("备份会话 journal 不完整尾行失败: %w", writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("关闭会话 journal 尾行备份失败: %w", closeErr)
		}
		if err := syncParentDirectory(backupPath); err != nil {
			return fmt.Errorf("同步会话 journal 尾行备份目录失败: %w", err)
		}
		return nil
	}
	return fmt.Errorf("无法为会话 journal 不完整尾行分配备份文件: %s", filePath)
}
