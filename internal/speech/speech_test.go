package speech

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"denova/config"
)

func TestSynthesizeCompatibleRequestAndOptionalAuthentication(t *testing.T) {
	for _, key := range []string{"", "test-secret"} {
		t.Run(key, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/custom/audio/speech" {
					t.Errorf("unexpected endpoint: %s %s", r.Method, r.URL.Path)
				}
				wantAuth := ""
				if key != "" {
					wantAuth = "Bearer " + key
				}
				if r.Header.Get("Authorization") != wantAuth {
					t.Error("incorrect authentication")
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if len(body) != 4 || body["model"] != "local-tts" || body["voice"] != "speaker/one" || body["input"] != "你好。" || body["response_format"] != "mp3" {
					t.Errorf("unexpected request: %#v", body)
				}
				w.Write([]byte("ID3audio"))
			}))
			defer server.Close()
			data, err := Synthesize(context.Background(), config.SpeechSettings{Endpoint: server.URL + "/custom/audio/speech", APIKey: key, Model: "local-tts", Voice: "speaker/one"}, "你好。")
			if err != nil || string(data) != "ID3audio" {
				t.Fatalf("audio=%q error=%v", data, err)
			}
		})
	}
}

func TestSynthesizeErrorsAreSafeAndRequestsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   Error
	}{
		{401, "secret upstream detail", Auth}, {403, "", Auth}, {404, "", ModelVoice}, {429, "", RateLimit}, {500, "", Service}, {200, "", Audio}, {200, `{"secret":"value"}`, Audio},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
		_, err := Synthesize(context.Background(), config.SpeechSettings{Endpoint: server.URL, Model: "m", Voice: "v"}, "text")
		server.Close()
		if !errors.Is(err, tc.want) {
			t.Fatalf("status %d: got %v, want %v", tc.status, err, tc.want)
		}
	}
	_, err := Synthesize(context.Background(), config.SpeechSettings{Endpoint: "file:///private", Model: "m", Voice: "v"}, "text")
	if !errors.Is(err, InvalidURL) {
		t.Fatal(err)
	}
	_, err = Synthesize(context.Background(), config.SpeechSettings{Endpoint: "http://127.0.0.1", Model: "m", Voice: "v"}, strings.Repeat("界", 4097))
	if !errors.Is(err, InvalidInput) {
		t.Fatal(err)
	}
}

func TestSynthesizeCancellationReachesProvider(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
			close(cancelled)
		case <-time.After(time.Second):
		}
	}))
	defer server.Close()
	var requests Requests
	const id = "speech-request-cancellation"
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- errors.New("synthesis panicked")
			}
		}()
		_, err := requests.Synthesize(context.Background(), id, config.SpeechSettings{Endpoint: server.URL, Model: "m", Voice: "v"}, "text")
		done <- err
	}()
	<-started
	requests.Cancel(id)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("provider request was not cancelled")
	}
}

func TestCancelBeforePostPreventsProviderRequest(t *testing.T) {
	var requests Requests
	const id = "speech-request-cancelled-before-post"
	requests.Cancel(id)
	_, err := requests.Synthesize(context.Background(), id, config.SpeechSettings{Endpoint: "http://127.0.0.1:1", Model: "m", Voice: "v"}, "text")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request started: %v", err)
	}
}
