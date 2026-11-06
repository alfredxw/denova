package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	hertzapp "github.com/cloudwego/hertz/pkg/app"

	"denova/internal/api/sse"
	apptask "denova/internal/app/task"
)

func TestServerShutdownDrainsSubscriptionsAndFinishesRequests(t *testing.T) {
	application := newTestApplication(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	server := NewServerWithListener(application, "0", listener)
	task, err := apptask.NewDeferred(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(task.Finish)
	server.engine.GET("/api/shutdown/game/stream", func(ctx context.Context, c *hertzapp.RequestContext) {
		sse.StreamTask(ctx, c, task)
	})
	server.engine.GET("/api/shutdown/writing/stream", func(ctx context.Context, c *hertzapp.RequestContext) {
		sse.StreamTaskUI(ctx, c, task)
	})
	requestStarted := make(chan struct{})
	server.engine.GET("/api/shutdown/save", func(ctx context.Context, c *hertzapp.RequestContext) {
		close(requestStarted)
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			c.String(http.StatusInternalServerError, "request canceled")
			return
		}
		c.String(http.StatusOK, "saved")
	})
	stop := make(chan struct{})
	server.engine.SetCustomSignalWaiter(func(errors chan error) error {
		select {
		case <-stop:
			return nil
		case err := <-errors:
			return err
		}
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Errorf("server panic: %v", recovered)
			}
		}()
		server.Run()
	}()
	t.Cleanup(func() {
		select {
		case <-stop:
		default:
			close(stop)
		}
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Error("server did not stop")
		}
	})
	// Real browsers and the desktop proxy negotiate compression. An omitted
	// SSE exclusion buffers forever instead of delivering initial events.
	transport := &http.Transport{}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 8 * time.Second}
	baseURL := "http://" + listener.Addr().String()
	var streams []*http.Response
	for _, path := range []string{
		"/api/projects/" + application.ProjectID() + "/events",
		"/api/shutdown/game/stream",
		"/api/shutdown/writing/stream",
	} {
		response, err := client.Get(baseURL + path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = response.Body.Close() })
		streams = append(streams, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("stream %s: status %d", path, response.StatusCode)
		}
	}
	requestDone := make(chan error, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				requestDone <- fmt.Errorf("request panic: %v", recovered)
			}
		}()
		response, err := client.Get(baseURL + "/api/shutdown/save")
		if err != nil {
			requestDone <- err
			return
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err == nil && (response.StatusCode != http.StatusOK || string(body) != "saved") {
			err = fmt.Errorf("incomplete response: status=%d body=%q", response.StatusCode, body)
		}
		requestDone <- err
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not start")
	}
	started := time.Now()
	close(stop)
	<-done
	elapsed := time.Since(started)
	t.Logf("HTTP shutdown with workspace, Game and Writing streams: %s", elapsed)
	if elapsed > time.Second {
		t.Errorf("HTTP shutdown waited for subscriptions: %s", elapsed)
	}
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	for _, stream := range streams {
		body, err := io.ReadAll(stream.Body)
		if err != nil {
			t.Fatalf("stream %s did not end cleanly: %v", stream.Request.URL.Path, err)
		}
		if strings.Contains(string(body), `"type":"finish"`) {
			t.Fatalf("shutdown incorrectly completed the Agent turn: %s", body)
		}
	}
	if snapshot := task.Snapshot(); snapshot.Finished || snapshot.CancelRequested {
		t.Fatalf("disconnect changed task execution: %+v", snapshot)
	}
	started = time.Now()
	application.Close()
	t.Logf("Application cleanup: %s", time.Since(started))
}
