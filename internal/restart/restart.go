package restart

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// DefaultDelay gives the HTTP response enough time to flush before replacing
// the current process.
const DefaultDelay = 500 * time.Millisecond

// Invocation describes the exact process image used for a Nova restart.
type Invocation struct {
	Executable string
	Args       []string
	Env        []string
}

// Scheduler schedules a process replacement. Tests can replace the boundary
// functions without touching the real process.
type Scheduler struct {
	Delay      time.Duration
	Invocation func() (Invocation, error)
	Sleep      func(time.Duration)
	Replace    func(Invocation) error
	Logf       func(string, ...any)
}

// Schedule validates the restart target immediately, then replaces the process
// asynchronously after the configured delay.
func (s Scheduler) Schedule() error {
	invocationFn := s.Invocation
	if invocationFn == nil {
		invocationFn = CurrentProcessInvocation
	}
	invocation, err := invocationFn()
	if err != nil {
		return err
	}

	delay := s.Delay
	if delay <= 0 {
		delay = DefaultDelay
	}
	sleep := s.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	replace := s.Replace
	if replace == nil {
		replace = ReplaceProcess
	}
	logf := s.Logf
	if logf == nil {
		logf = log.Printf
	}

	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				logf("[restart] panic recovered err=%v", recovered)
			}
		}()
		logf("[restart] scheduled executable=%s args=%d delay=%s", invocation.Executable, len(invocation.Args), delay)
		sleep(delay)
		if err := replace(invocation); err != nil {
			logf("[restart] failed executable=%s err=%v", invocation.Executable, err)
		}
	}()
	return nil
}

// ScheduleCurrentProcess restarts Nova with the current executable, arguments
// and environment.
func ScheduleCurrentProcess(delay time.Duration) error {
	return Scheduler{Delay: delay}.Schedule()
}

// CurrentProcessInvocation builds a bounded, explicit process invocation from
// the current executable, arguments and environment.
func CurrentProcessInvocation() (Invocation, error) {
	executable, err := os.Executable()
	if err != nil {
		return Invocation{}, fmt.Errorf("resolve executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return Invocation{}, fmt.Errorf("resolve executable path: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return Invocation{}, fmt.Errorf("stat executable %q: %w", executable, err)
	}
	if info.IsDir() {
		return Invocation{}, fmt.Errorf("executable path is a directory: %s", executable)
	}

	args := make([]string, 0, len(os.Args))
	args = append(args, executable)
	if len(os.Args) > 1 {
		args = append(args, os.Args[1:]...)
	}
	return Invocation{
		Executable: executable,
		Args:       args,
		Env:        append([]string(nil), os.Environ()...),
	}, nil
}
