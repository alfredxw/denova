// Package codex translates the App Server protocol to internal external runtime
// contracts. It owns disposable threads; none of its state is recovery truth.
package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
)

const maxProtocolMessageBytes = 16 << 20

var ErrDisconnected = errors.New("App Server connection closed")

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (err *rpcError) Error() string {
	return fmt.Sprintf("App Server RPC error %d: %s", err.Code, err.Message)
}

type packet struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type subscription struct {
	events chan packet
	done   chan struct{}
	once   sync.Once
	err    error
}

func (sub *subscription) fail(err error) {
	sub.once.Do(func() { sub.err = err; close(sub.done) })
}

// Client multiplexes a Denova-owned process. Close interrupts all attempts;
// cancelling an individual Run interrupts only its disposable thread.
type Client struct {
	mu            sync.Mutex
	pending       map[string]chan packet
	threads       map[string]*subscription
	out           chan packet
	done          chan struct{}
	err           error
	once          sync.Once
	next          atomic.Uint64
	stop          func()
	workers       sync.WaitGroup
	cwd           string
	version       string
	apiModel      string
	accountStatus string
}

func newClient(reader io.Reader, writer io.Writer, stop func()) *Client {
	c := &Client{pending: make(map[string]chan packet), threads: make(map[string]*subscription), out: make(chan packet, 64), done: make(chan struct{}), stop: stop}
	c.workers.Add(2)
	go c.worker("read protocol", func() {
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 64<<10), maxProtocolMessageBytes)
		for scanner.Scan() {
			var msg packet
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				c.fail(fmt.Errorf("decode App Server packet: %w", err))
				return
			}
			c.dispatch(msg)
		}
		if err := scanner.Err(); err != nil {
			c.fail(fmt.Errorf("read App Server protocol: %w", err))
			return
		}
		c.fail(ErrDisconnected)
	})
	go c.worker("write protocol", func() {
		encoder := json.NewEncoder(writer)
		for {
			select {
			case <-c.done:
				return
			case msg := <-c.out:
				if err := encoder.Encode(msg); err != nil {
					c.fail(fmt.Errorf("write App Server protocol: %w", err))
					return
				}
			}
		}
	})
	return c
}

func (c *Client) worker(operation string, work func()) {
	defer c.workers.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("[external-runtime] protocol worker panicked", "operation", operation, "panic", recovered)
			c.fail(fmt.Errorf("App Server %s panicked", operation))
		}
	}()
	work()
}

func (c *Client) fail(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		close(c.done)
		c.mu.Unlock()
		c.stop()
	})
}

func (c *Client) Close() error {
	c.fail(ErrDisconnected)
	c.workers.Wait()
	return nil
}

func (c *Client) send(ctx context.Context, msg packet) error {
	select {
	case <-c.done:
		return c.err
	case <-ctx.Done():
		return ctx.Err()
	case c.out <- msg:
		return nil
	}
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return err
	}
	id := json.RawMessage(fmt.Sprintf(`"denova-%d"`, c.next.Add(1)))
	response := make(chan packet, 1)
	c.mu.Lock()
	c.pending[string(id)] = response
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, string(id)); c.mu.Unlock() }()
	if err := c.send(ctx, packet{ID: id, Method: method, Params: body}); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.err
	case reply := <-response:
		if reply.Error != nil {
			return reply.Error
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(reply.Result, result); err != nil {
			return fmt.Errorf("decode App Server %s response: %w", method, err)
		}
		return nil
	}
}

func (c *Client) dispatch(msg packet) {
	if msg.Method == "" {
		c.mu.Lock()
		response := c.pending[string(msg.ID)]
		c.mu.Unlock()
		if response != nil {
			select {
			case response <- msg:
			default:
			}
		}
		return
	}
	if len(msg.ID) == 0 && c.accountNotification(msg) {
		return
	}
	var routing struct {
		ThreadID string `json:"threadId"`
	}
	if err := json.Unmarshal(msg.Params, &routing); err != nil {
		c.fail(fmt.Errorf("decode App Server notification routing: %w", err))
		return
	}
	c.mu.Lock()
	sub := c.threads[routing.ThreadID]
	c.mu.Unlock()
	if sub != nil {
		select {
		case <-sub.done:
		case sub.events <- msg:
			return
		default:
			sub.fail(errors.New("App Server notification backlog exceeds capacity"))
		}
	}
	if len(msg.ID) != 0 {
		// Late callbacks cannot escape to a different Session after unbinding.
		select {
		case c.out <- packet{ID: msg.ID, Error: &rpcError{Code: -32601, Message: "No active host for this request"}}:
		default:
			c.fail(errors.New("App Server response backlog exceeds capacity"))
		}
	}
}

func (c *Client) subscribe(threadID string) (*subscription, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.threads[threadID]; exists {
		return nil, nil, errors.New("App Server thread already bound")
	}
	sub := &subscription{events: make(chan packet, 256), done: make(chan struct{})}
	c.threads[threadID] = sub
	return sub, func() {
		c.mu.Lock()
		delete(c.threads, threadID)
		c.mu.Unlock()
		sub.fail(ErrDisconnected)
	}, nil
}
