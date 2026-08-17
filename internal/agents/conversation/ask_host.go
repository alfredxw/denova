package conversation

// HostAskAnswer is the transport-neutral answer accepted from an interactive
// host. Public Agent Interaction validation and durable resolution remain the
// only authority behind this transport DTO.
type HostAskAnswer struct {
	QuestionID        string   `json:"question_id"`
	SelectedOptionIDs []string `json:"selected_option_ids,omitempty"`
	CustomInput       string   `json:"custom_input,omitempty"`
}

type HostAskSelectedOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type HostAskAnswerResult struct {
	QuestionID      string                  `json:"question_id"`
	Question        string                  `json:"question"`
	SelectedOptions []HostAskSelectedOption `json:"selected_options,omitempty"`
	CustomInput     string                  `json:"custom_input,omitempty"`
}

// HostAskResolution is the stable answer/cancellation result exposed to any
// application host without leaking Session persistence types.
type HostAskResolution struct {
	Schema       string                `json:"schema"`
	ID           string                `json:"id"`
	Status       string                `json:"status"`
	Answers      []HostAskAnswerResult `json:"answers,omitempty"`
	CancelReason string                `json:"cancel_reason,omitempty"`
}
