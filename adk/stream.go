package adk

import (
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"sync"
)

var (
	// ErrNoValue tells a converting stream to drop the current item.
	ErrNoValue = errors.New("no value")
	// ErrRecvAfterClosed reports use of a reader after Close.
	ErrRecvAfterClosed = errors.New("recv after stream closed")
)

// PanicError converts a recovered goroutine or extension panic into an error.
type PanicError struct {
	Value any
	Stack []byte
}

func (err *PanicError) Error() string {
	return fmt.Sprintf("panic recovered: %v", err.Value)
}

func recoveredPanic(value any) *PanicError {
	return &PanicError{Value: value, Stack: append([]byte(nil), debug.Stack()...)}
}

type streamItem[T any] struct {
	value T
	err   error
}

type pipeState[T any] struct {
	mu           sync.Mutex
	changed      *sync.Cond
	items        []streamItem[T]
	capacity     int
	writerClosed bool
	readerClosed bool
}

func newPipeState[T any](capacity int) *pipeState[T] {
	state := &pipeState[T]{capacity: capacity}
	if capacity == 0 {
		// A one-element handoff preserves the useful behavior of an unbuffered
		// pipe without holding locks across a sender/receiver rendezvous.
		state.capacity = 1
	}
	state.changed = sync.NewCond(&state.mu)
	return state
}

func (state *pipeState[T]) send(value T, err error) bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	for state.capacity >= 0 && len(state.items) >= state.capacity && !state.readerClosed && !state.writerClosed {
		state.changed.Wait()
	}
	if state.readerClosed || state.writerClosed {
		return true
	}
	state.items = append(state.items, streamItem[T]{value: value, err: err})
	state.changed.Broadcast()
	return false
}

func (state *pipeState[T]) recv() (T, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	for len(state.items) == 0 && !state.writerClosed && !state.readerClosed {
		state.changed.Wait()
	}
	var zero T
	if state.readerClosed {
		return zero, ErrRecvAfterClosed
	}
	if len(state.items) == 0 {
		return zero, io.EOF
	}
	item := state.items[0]
	var empty streamItem[T]
	state.items[0] = empty
	state.items = state.items[1:]
	state.changed.Broadcast()
	return item.value, item.err
}

func (state *pipeState[T]) closeWriter() {
	state.mu.Lock()
	state.writerClosed = true
	state.changed.Broadcast()
	state.mu.Unlock()
}

func (state *pipeState[T]) closeReader() {
	state.mu.Lock()
	state.readerClosed = true
	state.items = nil
	state.changed.Broadcast()
	state.mu.Unlock()
}

// StreamWriter is the producer half of a Pipe.
type StreamWriter[T any] struct {
	state *pipeState[T]
	once  sync.Once
}

// Send appends a value/error pair. It returns true when the stream is closed.
func (writer *StreamWriter[T]) Send(value T, err error) bool {
	if writer == nil || writer.state == nil {
		return true
	}
	return writer.state.send(value, err)
}

// Close marks the producer complete. It is idempotent.
func (writer *StreamWriter[T]) Close() {
	if writer == nil || writer.state == nil {
		return
	}
	writer.once.Do(writer.state.closeWriter)
}

// StreamReader is a read-once stream. Call Close when abandoning it early.
type StreamReader[T any] struct {
	recvFn  func() (T, error)
	closeFn func()
	mu      sync.Mutex
	closed  bool
}

// Recv returns the next value, stream error, or io.EOF.
func (reader *StreamReader[T]) Recv() (T, error) {
	reader.mu.Lock()
	closed := reader.closed
	reader.mu.Unlock()
	if closed {
		var zero T
		return zero, ErrRecvAfterClosed
	}
	return reader.recvFn()
}

// Close releases the reader and unblocks a connected writer.
func (reader *StreamReader[T]) Close() {
	if reader == nil {
		return
	}
	reader.mu.Lock()
	if reader.closed {
		reader.mu.Unlock()
		return
	}
	reader.closed = true
	reader.mu.Unlock()
	if reader.closeFn != nil {
		reader.closeFn()
	}
}

// Pipe creates a stream. A negative capacity selects an unbounded buffer.
func Pipe[T any](capacity int) (*StreamReader[T], *StreamWriter[T]) {
	state := newPipeState[T](capacity)
	return &StreamReader[T]{recvFn: state.recv, closeFn: state.closeReader}, &StreamWriter[T]{state: state}
}

type arrayReader[T any] struct {
	mu     sync.Mutex
	values []T
	index  int
}

func (reader *arrayReader[T]) recv() (T, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.index >= len(reader.values) {
		var zero T
		return zero, io.EOF
	}
	value := reader.values[reader.index]
	reader.index++
	return value, nil
}

// StreamReaderFromArray returns a reader over a snapshot of values.
func StreamReaderFromArray[T any](values []T) *StreamReader[T] {
	array := &arrayReader[T]{values: append([]T(nil), values...)}
	return &StreamReader[T]{recvFn: array.recv}
}

type convertOptions struct {
	errWrapper func(error) error
	onEOF      func() (any, error)
}

// ConvertOption configures StreamReaderWithConvert.
type ConvertOption func(*convertOptions)

// WithErrWrapper transforms upstream stream errors. Returning nil drops the
// error and continues reading.
func WithErrWrapper(wrapper func(error) error) ConvertOption {
	return func(options *convertOptions) {
		options.errWrapper = wrapper
	}
}

// WithOnEOF injects one final value or error when the source reaches EOF.
func WithOnEOF(callback func() (any, error)) ConvertOption {
	return func(options *convertOptions) {
		options.onEOF = callback
	}
}

// StreamReaderWithConvert maps and optionally filters a stream.
func StreamReaderWithConvert[T, D any](source *StreamReader[T], convert func(T) (D, error), opts ...ConvertOption) *StreamReader[D] {
	options := &convertOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	var mu sync.Mutex
	eofHandled := false
	recv := func() (D, error) {
		for {
			value, err := source.Recv()
			if err != nil {
				var zero D
				if errors.Is(err, io.EOF) {
					mu.Lock()
					if options.onEOF != nil && !eofHandled {
						eofHandled = true
						callback := options.onEOF
						mu.Unlock()
						finalValue, finalErr := callback()
						if errors.Is(finalErr, io.EOF) {
							return zero, io.EOF
						}
						if finalValue == nil {
							return zero, finalErr
						}
						converted, ok := finalValue.(D)
						if !ok {
							return zero, fmt.Errorf("stream onEOF returned %T, want destination type", finalValue)
						}
						return converted, finalErr
					}
					mu.Unlock()
					return zero, io.EOF
				}
				if options.errWrapper != nil {
					err = options.errWrapper(err)
					if err == nil {
						continue
					}
				}
				return zero, err
			}
			converted, err := convert(value)
			if errors.Is(err, ErrNoValue) {
				continue
			}
			return converted, err
		}
	}
	return &StreamReader[D]{recvFn: recv, closeFn: source.Close}
}

// StreamReaderWithOnEOF is a typed convenience wrapper around WithOnEOF.
func StreamReaderWithOnEOF[T any](source *StreamReader[T], callback func() (T, error)) *StreamReader[T] {
	return StreamReaderWithConvert(source, func(value T) (T, error) { return value, nil }, WithOnEOF(func() (any, error) {
		return callback()
	}))
}

// StreamReaderWithErrWrapper is a convenience wrapper for upstream errors.
func StreamReaderWithErrWrapper[T any](source *StreamReader[T], wrapper func(error) error) *StreamReader[T] {
	return StreamReaderWithConvert(source, func(value T) (T, error) { return value, nil }, WithErrWrapper(wrapper))
}

// Copy fans a reader out into independent, unbounded readers. The original
// reader must not be consumed after Copy.
func (reader *StreamReader[T]) Copy(count int) []*StreamReader[T] {
	if count <= 0 {
		return nil
	}
	readers := make([]*StreamReader[T], count)
	writers := make([]*StreamWriter[T], count)
	for index := range count {
		readers[index], writers[index] = Pipe[T](-1)
	}
	safeGo(func() {
		defer reader.Close()
		for {
			value, err := reader.Recv()
			if errors.Is(err, io.EOF) {
				for _, writer := range writers {
					writer.Close()
				}
				return
			}
			for _, writer := range writers {
				writer.Send(value, err)
			}
			if err != nil {
				for _, writer := range writers {
					writer.Close()
				}
				return
			}
		}
	}, func(err error) {
		var zero T
		for _, writer := range writers {
			writer.Send(zero, err)
			writer.Close()
		}
	})
	return readers
}

// ConcatMessageStream drains and strictly merges a message stream.
func ConcatMessageStream(stream *StreamReader[*Message]) (*Message, error) {
	defer stream.Close()
	assembler := NewMessageAssembler()
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return assembler.Message()
		}
		if err != nil {
			return nil, err
		}
		if err := assembler.Append(message); err != nil {
			return nil, err
		}
	}
}

func safeGo(run func(), onPanic func(error)) {
	go func() {
		defer func() {
			if value := recover(); value != nil {
				func() {
					defer func() { _ = recover() }()
					onPanic(recoveredPanic(value))
				}()
			}
		}()
		run()
	}()
}
