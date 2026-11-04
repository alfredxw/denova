package desktop

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// backend owns the private control pipe. Business state and task admission
// remain in the child; closing this pipe asks it to drain and flush journals.
type backend struct {
	url, token, locale string
	cmd                *exec.Cmd
	in                 io.WriteCloser
	replies            chan backendReply
	done               chan struct{}
	requests           sync.Mutex
	stopping           atomic.Bool
	err                error
	nextID             int
}

type backendReply struct {
	Type   string          `json:"type"`
	ID     int             `json:"id"`
	URL    string          `json:"url"`
	Locale string          `json:"locale"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type activity struct {
	Active    bool                           `json:"active_tasks"`
	Scheduled bool                           `json:"scheduled_tasks"`
	Policy    struct{ Check, Download bool } `json:"policy"`
}

func startBackend(resources, userData, dataDir, origin string, development bool) (*backend, error) {
	name := "denova"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	b := &backend{token: hex.EncodeToString(randomBytes()), replies: make(chan backendReply, 8), done: make(chan struct{})}
	args := []string{}
	if development {
		args = append(args, "--dev-mode")
	}
	b.cmd = exec.Command(filepath.Join(resources, "backend", name), args...)
	b.cmd.Dir = userData
	b.cmd.Env = append(os.Environ(), "DENOVA_DIR="+dataDir)
	b.cmd.Stderr = backendLog{}
	hideChildWindow(b.cmd)
	var err error
	b.in, err = b.cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := b.cmd.StdoutPipe()
	if err != nil {
		_ = b.in.Close()
		return nil, err
	}
	if err = b.cmd.Start(); err != nil {
		_ = b.in.Close()
		return nil, err
	}
	go func() {
		defer close(b.done)
		defer close(b.replies)
		defer func() {
			if p := recover(); p != nil {
				slog.Error("Desktop control reader panicked", "error", p)
				_ = b.in.Close()
				b.err = fmt.Errorf("control reader panic: %v", p)
			}
			if err := b.cmd.Wait(); err != nil {
				b.err = err
			}
		}()
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			var reply backendReply
			if err := json.Unmarshal(scanner.Bytes(), &reply); err != nil {
				slog.Error("Invalid desktop backend reply", "error", err)
				continue
			}

			b.replies <- reply
		}
		if err := scanner.Err(); err != nil {
			b.err = err
		}
	}()
	if err := json.NewEncoder(b.in).Encode(map[string]any{"type": "start", "token": b.token, "origin": origin}); err != nil {
		b.stop()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	select {
	case reply, ok := <-b.replies:
		if !ok || reply.Type != "ready" {
			b.stop()
			return nil, errors.New("backend exited before readiness")
		}
		b.url, b.locale = reply.URL, reply.Locale
	case <-ctx.Done():
		b.stop()
		return nil, ctx.Err()
	}
	return b, nil
}

func randomBytes() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func (b *backend) request(ctx context.Context, kind string, result any) error {
	b.requests.Lock()
	defer b.requests.Unlock()
	if b.stopping.Load() {
		return errors.New("backend is stopping")
	}
	b.nextID++
	if err := json.NewEncoder(b.in).Encode(map[string]any{"id": b.nextID, "type": kind}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		select {
		case reply, ok := <-b.replies:
			if !ok {
				return errors.New("backend exited")
			}
			if reply.ID != b.nextID {
				continue
			}
			if reply.Error != "" {
				return errors.New(reply.Error)
			}
			return json.Unmarshal(reply.Result, result)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (b *backend) stop() {
	if b == nil {
		return
	}
	if !b.stopping.Swap(true) {
		_ = b.in.Close()
	}
	<-b.done
}

// Child diagnostics go to the host log, even after its launcher disconnects.
type backendLog struct{}

func (backendLog) Write(data []byte) (int, error) {
	slog.Info("Desktop backend", "message", strings.TrimSpace(string(data)))
	return len(data), nil
}
