package external

import "context"

// Model IDs and effort values are engine-owned. They are neither Native model
// profile IDs nor a globally interchangeable set of reasoning settings.
type Model struct {
	ID            string   `json:"id"`
	DisplayName   string   `json:"display_name"`
	Efforts       []string `json:"efforts"`
	DefaultEffort string   `json:"default_effort,omitempty"`
}

type Models struct {
	Items     []Model `json:"items"`
	DefaultID string  `json:"default_id,omitempty"`
}

type ConnectionState struct {
	Status    string
	ReasonKey string
}

// Connection inspects host-local credentials managed by the engine's CLI.
// Status is cached and never probes. Account/model methods perform bounded
// infrastructure I/O; a Run remains cancellable without a total time limit.
type Connection interface {
	Adapter
	Status() ConnectionState
	Check(context.Context) (ConnectionState, error)
	Models(context.Context) (Models, error)
	Close() error
}
