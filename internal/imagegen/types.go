package imagegen

import (
	"context"

	"nova/config"
)

type GenerateRequest struct {
	ProfileID    string `json:"profile_id,omitempty"`
	Prompt       string `json:"prompt"`
	N            int    `json:"n,omitempty"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
}

type Result struct {
	ProfileID    string
	Provider     string
	Model        string
	Created      int64
	Size         string
	Quality      string
	OutputFormat string
	Images       []Image
}

type Image struct {
	Data          []byte
	MIMEType      string
	Extension     string
	RevisedPrompt string
	SourceURL      string
}

type Adapter interface {
	Generate(ctx context.Context, profile config.ResolvedImageAPIProfile, request GenerateRequest) (Result, error)
}
