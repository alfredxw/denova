package handlers

import novaApp "nova/internal/app"

// Handlers owns HTTP request handlers and adapts requests to application services.
type Handlers struct {
	app *novaApp.App
}

// New creates a handler set bound to one application runtime.
func New(application *novaApp.App) *Handlers {
	return &Handlers{app: application}
}
