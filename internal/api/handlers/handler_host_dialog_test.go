package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	hertzserver "github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"denova/internal/hostdialog"
)

type fakeDirectoryPicker struct {
	selection hostdialog.DirectorySelection
	err       error
	options   hostdialog.DirectoryOptions
}

func (picker *fakeDirectoryPicker) SelectDirectory(_ context.Context, options hostdialog.DirectoryOptions) (hostdialog.DirectorySelection, error) {
	picker.options = options
	return picker.selection, picker.err
}

func TestHandleDirectoryPickerReturnsSelectionAndLocalizedTitle(t *testing.T) {
	picker := &fakeDirectoryPicker{selection: hostdialog.DirectorySelection{Path: "/projects/story"}}
	response := performDirectoryPickerRequest(t, &Handlers{directoryPicker: picker}, map[string]string{
		"initial_path": "/projects/old",
	}, "en-US")
	if response.Code != http.StatusOK {
		t.Fatalf("directory picker status = %d body=%s", response.Code, response.Body.String())
	}
	var selection hostdialog.DirectorySelection
	if err := json.Unmarshal(response.Body.Bytes(), &selection); err != nil {
		t.Fatal(err)
	}
	if selection.Path != "/projects/story" || selection.Canceled {
		t.Fatalf("unexpected directory selection: %#v", selection)
	}
	if picker.options.Title != "Choose a project folder" || picker.options.InitialPath != "/projects/old" {
		t.Fatalf("unexpected picker options: %#v", picker.options)
	}
}

func TestHandleDirectoryPickerTreatsCancellationAsSuccess(t *testing.T) {
	response := performDirectoryPickerRequest(t, &Handlers{directoryPicker: &fakeDirectoryPicker{
		selection: hostdialog.DirectorySelection{Canceled: true},
	}}, nil, "zh-CN")
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"canceled":true`)) {
		t.Fatalf("cancel response = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleDirectoryPickerReportsUnavailableHost(t *testing.T) {
	response := performDirectoryPickerRequest(t, &Handlers{directoryPicker: &fakeDirectoryPicker{
		err: hostdialog.ErrUnavailable,
	}}, nil, "en-US")
	if response.Code != http.StatusServiceUnavailable || !bytes.Contains(response.Body.Bytes(), []byte("unavailable")) {
		t.Fatalf("unavailable response = %d body=%s", response.Code, response.Body.String())
	}
}

func TestHandleDirectoryPickerReportsHostFailure(t *testing.T) {
	response := performDirectoryPickerRequest(t, &Handlers{directoryPicker: &fakeDirectoryPicker{
		err: errors.New("display disconnected"),
	}}, nil, "en-US")
	if response.Code != http.StatusInternalServerError || !bytes.Contains(response.Body.Bytes(), []byte("display disconnected")) {
		t.Fatalf("failure response = %d body=%s", response.Code, response.Body.String())
	}
}

func performDirectoryPickerRequest(t *testing.T, handlers *Handlers, body any, locale string) *ut.ResponseRecorder {
	t.Helper()
	server := hertzserver.Default()
	server.POST("/api/host/dialogs/directory", handlers.HandleDirectoryPicker)
	var requestBody *ut.Body
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = &ut.Body{Body: bytes.NewReader(raw), Len: len(raw)}
	}
	return ut.PerformRequest(
		server.Engine,
		http.MethodPost,
		"/api/host/dialogs/directory",
		requestBody,
		ut.Header{Key: "Content-Type", Value: "application/json"},
		ut.Header{Key: "X-Denova-Locale", Value: locale},
	)
}
