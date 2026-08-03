// Package llmhttp owns the default transport policy shared by model protocol
// adapters.
package llmhttp

import "net/http"

// Client returns the caller-owned client when supplied. The fallback has no
// overall or response-header timeout; model execution is bounded only by the
// caller context, as required by Denova's long-running Agent contract.
func Client(configured *http.Client) *http.Client {
	if configured != nil {
		return configured
	}
	return &http.Client{}
}
