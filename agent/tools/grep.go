package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode/utf8"
)

// Grep compiles and executes one controlled rg command, then applies a stable
// cursor to complete logical records. The model-visible command never reaches
// a shell, and the implementation stops the process as soon as one additional
// record proves that the bounded page is partial.
func (workspace *LocalWorkspace) Grep(ctx context.Context, request GrepRequest) (SearchResult, error) {
	if workspace == nil || workspace.Root() == "" {
		return SearchResult{}, errors.New("grep workspace is not configured / grep 工作区未配置")
	}
	request = normalizeGrepRequest(request)
	command, err := workspace.compileGrepCommand(request.Command)
	if err != nil {
		return SearchResult{}, err
	}
	cursor, err := decodeGrepCursor(request.Cursor, request)
	if err != nil {
		return SearchResult{}, err
	}
	limits := workspace.Limits()
	limit := limits.DefaultDirectoryItems
	capacity := min(limit, limits.MaxResultBytes)
	if capacity < limits.MaxResultBytes {
		capacity++
	}
	seen := make(map[string]struct{}, capacity)
	entries := make([]string, 0, min(limit, capacity))
	eligible, outputBytes := 0, 0
	truncated := false
	oversizedEntry := false
	prefix := ""
	prefixVerified := cursor.Offset == 0
	cursorStale := false

	consume := func(entry string) bool {
		entry = normalizeGrepEntry(entry)
		if _, duplicate := seen[entry]; duplicate {
			return true
		}
		seen[entry] = struct{}{}
		if eligible < cursor.Offset {
			eligible++
			prefix = advanceGrepPrefix(prefix, []string{entry})
			if eligible == cursor.Offset {
				prefixVerified = prefix == cursor.Prefix
				if !prefixVerified {
					cursorStale = true
					return false
				}
			}
			return true
		}
		if len(entries) >= limit {
			truncated = true
			return false
		}
		additional := len(entry)
		if len(entries) > 0 {
			additional++
		}
		if outputBytes+additional > limits.MaxResultBytes {
			oversizedEntry = len(entries) == 0
			truncated = true
			return false
		}
		entries = append(entries, entry)
		outputBytes += additional
		eligible++
		return true
	}

	stopped, err := workspace.runGrepCommand(ctx, command, limits.MaxResultBytes, consume)
	if err != nil {
		return SearchResult{}, err
	}
	if cursorStale || !prefixVerified {
		return SearchResult{}, errors.New("grep cursor is stale because preceding results changed; rerun without cursor / grep 游标已因前序结果变化而失效；请移除 cursor 后重新搜索")
	}
	if stopped {
		truncated = true
	}
	if oversizedEntry {
		return SearchResult{}, fmt.Errorf("grep result block exceeds the %d-byte result limit; reduce context or narrow the command / grep 结果块超过 %d 字节上限；请减少上下文或缩小命令范围", limits.MaxResultBytes, limits.MaxResultBytes)
	}
	result := SearchResult{Entries: entries, Truncated: truncated, Warnings: command.warnings}
	if truncated {
		next := grepCursorState{
			Offset: cursor.Offset + len(entries),
			Prefix: advanceGrepPrefix(cursor.Prefix, entries),
		}
		encoded, err := encodeGrepCursor(next, request)
		if err != nil {
			return SearchResult{}, fmt.Errorf("encode grep cursor: %w", err)
		}
		result.NextCursor = encoded
	}
	return result, nil
}

func (workspace *LocalWorkspace) runGrepCommand(
	ctx context.Context,
	plan compiledGrepCommand,
	maxBytes int,
	consume func(string) bool,
) (bool, error) {
	if len(plan.stages) == 0 {
		return false, errors.New("grep execution plan has no stages / grep 执行计划不包含命令")
	}
	commands := make([]*exec.Cmd, len(plan.stages))
	stdoutPipes := make([]io.ReadCloser, len(plan.stages))
	stderrPipes := make([]io.ReadCloser, len(plan.stages))
	for index, stage := range plan.stages {
		command := exec.CommandContext(ctx, workspace.ripgrepExecutable, grepArguments(stage, index > 0)...)
		command.Dir = workspace.root
		stdout, err := command.StdoutPipe()
		if err != nil {
			for previous := 0; previous < index; previous++ {
				_ = stdoutPipes[previous].Close()
				_ = stderrPipes[previous].Close()
			}
			return false, fmt.Errorf("create grep pipeline stage %d stdout: %w", index+1, err)
		}
		stderr, err := command.StderrPipe()
		if err != nil {
			_ = stdout.Close()
			for previous := 0; previous < index; previous++ {
				_ = stdoutPipes[previous].Close()
				_ = stderrPipes[previous].Close()
			}
			return false, fmt.Errorf("create grep pipeline stage %d stderr: %w", index+1, err)
		}
		commands[index], stdoutPipes[index], stderrPipes[index] = command, stdout, stderr
		if index > 0 {
			command.Stdin = stdoutPipes[index-1]
		}
	}
	if err := workspace.verifyRootIdentity(); err != nil {
		for index := range commands {
			_ = stdoutPipes[index].Close()
			_ = stderrPipes[index].Close()
		}
		return false, err
	}
	diagnostics := make([]<-chan processDiagnostic, len(plan.stages))
	for index, stderr := range stderrPipes {
		diagnostics[index] = readProcessDiagnostics(stderr, maxBytes)
	}
	started := 0
	for index, command := range commands {
		if err := command.Start(); err != nil {
			for running := 0; running < started; running++ {
				_ = commands[running].Process.Kill()
			}
			for pipe := range commands {
				_ = stdoutPipes[pipe].Close()
				_ = stderrPipes[pipe].Close()
			}
			for running := 0; running < started; running++ {
				_ = commands[running].Wait()
			}
			for diagnostic := range diagnostics {
				<-diagnostics[diagnostic]
			}
			return false, fmt.Errorf("start ripgrep pipeline stage %d: %w", index+1, err)
		}
		started++
	}
	// Each downstream process has inherited its input descriptor after Start.
	// Closing the parent's copies lets an early downstream exit propagate
	// SIGPIPE instead of leaving an upstream rg blocked on a full pipe.
	for index := 0; index < len(stdoutPipes)-1; index++ {
		_ = stdoutPipes[index].Close()
	}
	stopped := false
	var scanErr error
	groupsContext := plan.groupsContext()
	var block strings.Builder
	stopPipeline := func() {
		stopped = true
		for _, command := range commands {
			_ = command.Process.Kill()
		}
	}
	emit := func(entry string) bool {
		if entry == "" {
			return true
		}
		if !consume(entry) {
			stopPipeline()
			return false
		}
		return true
	}
	appendBlockLine := func(line string) bool {
		additional := len(line)
		if block.Len() > 0 {
			additional++
		}
		if block.Len()+additional > maxBytes {
			scanErr = fmt.Errorf("grep context block exceeds the %d-byte result limit; reduce context or narrow the command / grep 上下文结果块超过 %d 字节上限；请减少上下文或缩小命令范围", maxBytes, maxBytes)
			stopPipeline()
			return false
		}
		if block.Len() > 0 {
			block.WriteByte('\n')
		}
		block.WriteString(line)
		return true
	}

	scanner := bufio.NewScanner(stdoutPipes[len(stdoutPipes)-1])
	scanner.Buffer(make([]byte, 64*1024), maxBytes+1)
	for scanner.Scan() {
		if err := contextError(ctx); err != nil {
			scanErr = err
			stopPipeline()
			break
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if !utf8.ValidString(line) {
			scanErr = errors.New("grep encountered non-UTF-8 output; select a compatible encoding or narrow the command / grep 遇到非 UTF-8 输出；请选择兼容编码或缩小命令范围")
			stopPipeline()
			break
		}
		if !groupsContext {
			if !emit(line) {
				break
			}
			continue
		}
		if line == "--" {
			if block.Len() == 0 {
				continue
			}
			if !appendBlockLine(line) || !emit(block.String()) {
				break
			}
			block.Reset()
			continue
		}
		if !appendBlockLine(line) {
			break
		}
	}
	if scanErr == nil && scanner.Err() != nil {
		scanErr = fmt.Errorf("read ripgrep output: %w", scanner.Err())
		stopPipeline()
	}
	if scanErr == nil && !stopped && block.Len() > 0 {
		_ = emit(block.String())
	}
	diagnosticResults := make([]processDiagnostic, len(commands))
	waitErrors := make([]error, len(commands))
	for index := len(commands) - 1; index >= 0; index-- {
		diagnosticResults[index] = <-diagnostics[index]
		waitErrors[index] = commands[index].Wait()
	}
	if scanErr != nil {
		return stopped, scanErr
	}
	for index, diagnostic := range diagnosticResults {
		if diagnostic.err != nil {
			return stopped, fmt.Errorf("read ripgrep pipeline stage %d diagnostics: %w", index+1, diagnostic.err)
		}
		if waitErrors[index] != nil && !stopped {
			var exitError *exec.ExitError
			if !errors.As(waitErrors[index], &exitError) || exitError.ExitCode() != 1 {
				message := strings.TrimSpace(diagnostic.content)
				if message == "" {
					message = waitErrors[index].Error()
				}
				return false, fmt.Errorf("ripgrep pipeline stage %d failed / ripgrep 管道第 %d 段执行失败: %s", index+1, index+1, boundedString(message, maxBytes))
			}
		}
	}
	return stopped, nil
}

func normalizeGrepEntry(entry string) string {
	lines := strings.Split(entry, "\n")
	for index, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		line = strings.TrimPrefix(line, "./")
		line = strings.TrimPrefix(line, `.\`)
		lines[index] = line
	}
	return strings.Join(lines, "\n")
}

func normalizeGrepRequest(request GrepRequest) GrepRequest {
	request.Command = strings.TrimSpace(request.Command)
	request.Cursor = strings.TrimSpace(request.Cursor)
	return request
}
