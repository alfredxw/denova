package agentrun

// InputVisibility controls only transcript projection. Both variants remain
// canonical model context; model-only inputs are host-owned and cannot be set
// by transport JSON.
type InputVisibility string

const (
	InputVisible   InputVisibility = "visible"
	InputModelOnly InputVisibility = "model_only"
)
