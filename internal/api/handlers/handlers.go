package handlers

import novaApp "denova/internal/app"
import "denova/internal/hostdialog"
import "denova/internal/speech"

// Handlers owns HTTP request handlers and adapts requests to application services.
type Handlers struct {
	app             *novaApp.App
	directoryPicker hostdialog.DirectoryPicker
	pathRevealer    hostdialog.PathRevealer
	speechRequests  speech.Requests
}

// New creates a handler set bound to one application runtime.
func New(application *novaApp.App) *Handlers {
	return &Handlers{
		app:             application,
		directoryPicker: hostdialog.NewDirectoryPicker(),
		pathRevealer:    hostdialog.NewPathRevealer(),
	}
}
