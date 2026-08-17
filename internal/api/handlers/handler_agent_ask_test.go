package handlers

import (
	"errors"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"

	appsvc "denova/internal/app"
)

func TestAskResolutionErrorStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "unknown id", err: appsvc.ErrAgentAskNotFound, want: http.StatusNotFound},
		{name: "invalid answer", err: errors.New("invalid answer"), want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requestContext := app.NewContext(0)
			writeAskResolutionError(requestContext, test.err)
			if got := requestContext.Response.StatusCode(); got != test.want {
				t.Fatalf("status = %d, want %d", got, test.want)
			}
		})
	}
}
