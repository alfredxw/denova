package app

import (
	"errors"
	"testing"
)

func TestActiveTaskReplayAdmissionAllowsMultipleProductsWithinDefaultBudget(t *testing.T) {
	admission := activeTaskReplayAdmission{}
	configManagerTask, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	automationTask, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		configManagerTask.failBeforeStart(errors.New("test cleanup"))
		automationTask.failBeforeStart(errors.New("test cleanup"))
	})

	configReservation, err := admission.reserve(configManagerTask)
	if err != nil {
		t.Fatalf("reserve Config Manager replay: %v", err)
	}
	defer configReservation.release()
	automationReservation, err := admission.reserve(automationTask)
	if err != nil {
		t.Fatalf("reserve Automation replay concurrently: %v", err)
	}
	defer automationReservation.release()

	if got := admission.activeBytesLocked(); got != 2*configManagerTask.displayReplayRegistryCharge() {
		t.Fatalf("active replay bytes = %d, want two full Task charges", got)
	}
}

func TestActiveTaskReplayAdmissionRejectsBeforeExceedingProcessBudget(t *testing.T) {
	admission := activeTaskReplayAdmission{}
	tasks := make([]*Task, 0, maxActiveReplayTasks+1)
	reservations := make([]*activeTaskReplayReservation, 0, maxActiveReplayTasks)
	defer func() {
		for _, reservation := range reservations {
			reservation.release()
		}
		for _, task := range tasks {
			task.failBeforeStart(errors.New("test cleanup"))
		}
	}()

	for index := 0; index < maxActiveReplayTasks; index++ {
		task, err := NewDeferredRegisteredTask(nil)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		reservation, err := admission.reserve(task)
		if err != nil {
			t.Fatalf("reserve active Task %d: %v", index, err)
		}
		reservations = append(reservations, reservation)
	}
	overflow, err := NewDeferredRegisteredTask(nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks = append(tasks, overflow)
	if reservation, err := admission.reserve(overflow); reservation != nil || !errors.Is(err, ErrAgentReplayCapacity) {
		t.Fatalf("overflow reservation = %#v err=%v, want ErrAgentReplayCapacity", reservation, err)
	}
	if got := admission.activeBytesLocked(); got > defaultActiveReplayByteLimit {
		t.Fatalf("active replay budget exceeded: got=%d limit=%d", got, defaultActiveReplayByteLimit)
	}

	reservations[0].release()
	if replacement, err := admission.reserve(overflow); err != nil {
		t.Fatalf("reserve after release: %v", err)
	} else {
		replacement.release()
	}
}
