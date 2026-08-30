package book

import (
	"errors"
	"path/filepath"
	"strings"

	"denova/internal/portablepath"
)

// ValidateNewName 校验重命名目标文件名，避免通过文件名逃逸目录。
func ValidateNewName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("新名称不能为空")
	}
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) {
		return errors.New("新名称不能包含路径分隔符")
	}
	if strings.HasPrefix(name, ".") {
		return errors.New("不允许使用隐藏文件名")
	}
	return portablepath.ValidateComponent(name)
}
