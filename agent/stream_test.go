package agent

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestStreamPipeAndArray(t *testing.T) {
	reader, writer := Pipe[int](-1)
	if writer.Send(1, nil) || writer.Send(2, errors.New("two")) {
		t.Fatal("unbounded pipe unexpectedly closed")
	}
	writer.Close()

	value, err := reader.Recv()
	if value != 1 || err != nil {
		t.Fatalf("first recv = %d, %v", value, err)
	}
	value, err = reader.Recv()
	if value != 2 || err == nil || err.Error() != "two" {
		t.Fatalf("second recv = %d, %v", value, err)
	}
	if _, err := reader.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("end recv = %v", err)
	}
	reader.Close()
	if _, err := reader.Recv(); !errors.Is(err, ErrRecvAfterClosed) {
		t.Fatalf("recv after close = %v", err)
	}

	array := StreamReaderFromArray([]string{"a", "b"})
	defer array.Close()
	for _, want := range []string{"a", "b"} {
		got, err := array.Recv()
		if err != nil || got != want {
			t.Fatalf("array recv = %q, %v; want %q", got, err, want)
		}
	}
}

func TestStreamConvertFilterOnEOFAndErrorWrapper(t *testing.T) {
	source := StreamReaderFromArray([]int{0, 1, 2})
	converted := StreamReaderWithConvert(source, func(value int) (string, error) {
		if value == 0 {
			return "", ErrNoValue
		}
		return fmt.Sprintf("v%d", value), nil
	}, WithOnEOF(func() (any, error) {
		return "done", nil
	}))
	defer converted.Close()
	var got []string
	for {
		value, err := converted.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, value)
	}
	if !reflect.DeepEqual(got, []string{"v1", "v2", "done"}) {
		t.Fatalf("converted = %#v", got)
	}

	errorSource, errorWriter := Pipe[int](-1)
	errorWriter.Send(0, errors.New("skip"))
	errorWriter.Send(3, nil)
	errorWriter.Close()
	wrapped := StreamReaderWithErrWrapper(errorSource, func(err error) error {
		if err.Error() == "skip" {
			return nil
		}
		return fmt.Errorf("wrapped: %w", err)
	})
	defer wrapped.Close()
	value, err := wrapped.Recv()
	if err != nil || value != 3 {
		t.Fatalf("wrapped recv = %d, %v", value, err)
	}
}

func TestStreamCopyFansOutAndRecoversPanic(t *testing.T) {
	copies := StreamReaderFromArray([]int{1, 2, 3}).Copy(2)
	for index, copy := range copies {
		var got []int
		for {
			value, err := copy.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("copy %d: %v", index, err)
			}
			got = append(got, value)
		}
		copy.Close()
		if !reflect.DeepEqual(got, []int{1, 2, 3}) {
			t.Fatalf("copy %d = %#v", index, got)
		}
	}

	panicking := &StreamReader[int]{recvFn: func() (int, error) { panic("boom") }}
	panicCopy := panicking.Copy(1)[0]
	defer panicCopy.Close()
	if _, err := panicCopy.Recv(); err == nil {
		t.Fatal("expected recovered panic error")
	} else {
		var panicErr *PanicError
		if !errors.As(err, &panicErr) {
			t.Fatalf("panic error = %T: %v", err, err)
		}
	}
}
