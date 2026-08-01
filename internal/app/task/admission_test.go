package task

import (
	"errors"
	"testing"
)

func TestReplayAdmissionAllowsMultipleProductsWithinDefaultBudget(t *testing.T) {
	var admission ReplayAdmission
	configManagerTask, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	automationTask, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		configManagerTask.RejectStart(errors.New("test cleanup"))
		automationTask.RejectStart(errors.New("test cleanup"))
	})

	configReservation, err := admission.Reserve(configManagerTask)
	if err != nil {
		t.Fatalf("reserve Config Manager replay: %v", err)
	}
	defer configReservation.Release()
	automationReservation, err := admission.Reserve(automationTask)
	if err != nil {
		t.Fatalf("reserve Automation replay concurrently: %v", err)
	}
	defer automationReservation.Release()

	stats := admission.Stats()
	if want := 2 * configManagerTask.DisplayReplayCharge(); stats.ActiveTasks != 2 || stats.ActiveBytes != want {
		t.Fatalf("active replay stats = %#v, want 2 Tasks and %d bytes", stats, want)
	}
}

func TestReplayAdmissionRejectsBeforeExceedingProcessBudget(t *testing.T) {
	var admission ReplayAdmission
	tasks := make([]*Task, 0, DefaultMaxActiveReplayTasks+1)
	reservations := make([]*ReplayReservation, 0, DefaultMaxActiveReplayTasks)
	defer func() {
		for _, reservation := range reservations {
			reservation.Release()
		}
		for _, task := range tasks {
			task.RejectStart(errors.New("test cleanup"))
		}
	}()

	for index := 0; index < DefaultMaxActiveReplayTasks; index++ {
		task, err := NewDeferred(nil)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		reservation, err := admission.Reserve(task)
		if err != nil {
			t.Fatalf("reserve active Task %d: %v", index, err)
		}
		reservations = append(reservations, reservation)
	}
	overflow, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks = append(tasks, overflow)
	if reservation, err := admission.Reserve(overflow); reservation != nil || !errors.Is(err, ErrReplayCapacity) {
		t.Fatalf("overflow reservation = %#v err=%v, want ErrReplayCapacity", reservation, err)
	}
	if got := admission.Stats().ActiveBytes; got > DefaultActiveReplayByteLimit {
		t.Fatalf("active replay budget exceeded: got=%d limit=%d", got, DefaultActiveReplayByteLimit)
	}

	reservations[0].Release()
	if replacement, err := admission.Reserve(overflow); err != nil {
		t.Fatalf("reserve after release: %v", err)
	} else {
		replacement.Release()
	}
}

func TestReplayAdmissionUsesConfiguredLimits(t *testing.T) {
	var admission ReplayAdmission
	admission.Configure(ReplayAdmissionLimits{MaxActive: 1, MaxBytes: 1 << 30})
	first, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		first.RejectStart(errors.New("test cleanup"))
		second.RejectStart(errors.New("test cleanup"))
	})
	reservation, err := admission.Reserve(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reservation.Release()
	if _, err := admission.Reserve(second); !errors.Is(err, ErrReplayCapacity) {
		t.Fatalf("configured active limit error = %v, want ErrReplayCapacity", err)
	}
}
