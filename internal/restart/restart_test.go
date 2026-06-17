package restart

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestSchedulerInvokesReplacementAfterDelay(t *testing.T) {
	want := Invocation{
		Executable: "/tmp/nova",
		Args:       []string{"/tmp/nova", "--port", "8090"},
		Env:        []string{"NOVA_TEST=1"},
	}
	delay := 25 * time.Millisecond
	done := make(chan Invocation, 1)
	errs := make(chan string, 1)

	scheduler := Scheduler{
		Delay: delay,
		Invocation: func() (Invocation, error) {
			return want, nil
		},
		Sleep: func(got time.Duration) {
			if got != delay {
				errs <- "unexpected delay"
			}
		},
		Replace: func(got Invocation) error {
			done <- got
			return nil
		},
		Logf: func(string, ...any) {},
	}

	if err := scheduler.Schedule(); err != nil {
		t.Fatalf("Schedule returned error: %v", err)
	}

	select {
	case got := <-done:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("replacement invocation = %#v, want %#v", got, want)
		}
	case err := <-errs:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("replacement was not invoked")
	}
}

func TestSchedulerReturnsInvocationErrorBeforeScheduling(t *testing.T) {
	wantErr := errors.New("no executable")
	scheduler := Scheduler{
		Invocation: func() (Invocation, error) {
			return Invocation{}, wantErr
		},
		Sleep: func(time.Duration) {
			t.Fatal("sleep should not run when invocation validation fails")
		},
		Replace: func(Invocation) error {
			t.Fatal("replace should not run when invocation validation fails")
			return nil
		},
		Logf: func(string, ...any) {},
	}

	if err := scheduler.Schedule(); !errors.Is(err, wantErr) {
		t.Fatalf("Schedule error = %v, want %v", err, wantErr)
	}
}

func TestCurrentProcessInvocationUsesExecutableAsArg0(t *testing.T) {
	invocation, err := CurrentProcessInvocation()
	if err != nil {
		t.Fatalf("CurrentProcessInvocation returned error: %v", err)
	}
	if invocation.Executable == "" {
		t.Fatal("Executable is empty")
	}
	if len(invocation.Args) == 0 || invocation.Args[0] != invocation.Executable {
		t.Fatalf("Args[0] = %q, want executable %q", invocation.Args, invocation.Executable)
	}
	if invocation.Env == nil {
		t.Fatal("Env should be a bounded copy, not nil")
	}
}
