package update

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func replaceFile(target, source, backup string) error {
	if _, err := os.Stat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("备份当前可执行文件失败: %w", err)
		}
	}
	if err := copyFile(source, target, 0o755); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("替换可执行文件失败: %w", err)
	}
	return nil
}

func replaceDir(target, source, backup string) error {
	if _, err := os.Stat(source); err != nil {
		return nil
	}
	if _, err := os.Stat(target); err == nil {
		if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
			return err
		}
		if err := os.Rename(target, backup); err != nil {
			return fmt.Errorf("备份目录失败 target=%s err=%w", target, err)
		}
	}
	if err := copyDir(source, target); err != nil {
		_ = os.Rename(backup, target)
		return fmt.Errorf("替换目录失败 target=%s err=%w", target, err)
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDir(source, target string) error {
	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, dest, info.Mode().Perm())
	})
}

func windowsApplyScript(pid int, source, target, backup, exeName string) string {
	return fmt.Sprintf(`@echo off
setlocal
set "PID=%d"
set "SRC=%s"
set "DST=%s"
set "BACKUP=%s"
:wait
tasklist /FI "PID eq %%PID%%" | find "%%PID%%" >NUL
if not errorlevel 1 (
  timeout /t 1 /nobreak >NUL
  goto wait
)
if not exist "%%BACKUP%%" mkdir "%%BACKUP%%"
if exist "%%DST%%\%s" move /Y "%%DST%%\%s" "%%BACKUP%%\%s" >NUL
if exist "%%DST%%\web" rmdir /S /Q "%%DST%%\web"
if exist "%%DST%%\skills" rmdir /S /Q "%%DST%%\skills"
xcopy /E /I /Y "%%SRC%%\web" "%%DST%%\web" >NUL
xcopy /E /I /Y "%%SRC%%\skills" "%%DST%%\skills" >NUL
copy /Y "%%SRC%%\%s" "%%DST%%\%s" >NUL
copy /Y "%%SRC%%\README.md" "%%DST%%\README.md" >NUL 2>NUL
copy /Y "%%SRC%%\CHANGELOG.md" "%%DST%%\CHANGELOG.md" >NUL 2>NUL
copy /Y "%%SRC%%\LICENSE" "%%DST%%\LICENSE" >NUL 2>NUL
endlocal
`, pid, source, target, backup, exeName, exeName, exeName, exeName, exeName)
}
