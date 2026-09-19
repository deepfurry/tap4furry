package runtime

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/deepfurry/tap4furry/server/internal/transport/health"
	"github.com/deepfurry/tap4furry/server/internal/transport/public"
	"github.com/gofiber/fiber/v3"
)

func TestHTTPStopsOnCancellation(t *testing.T) {
	// Reserve an ephemeral address before starting the actual lifecycle helper.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker := health.New(func(context.Context) error { return nil }, func(context.Context) error { return nil })
	app := fiber.New()
	public.Register(app, checker, nil, nil, public.Options{Environment: "test", PublicOrigin: "http://localhost:4321"})
	done := make(chan error, 1)
	go func() { done <- HTTP(ctx, app, address, checker, slog.New(slog.NewJSONHandler(io.Discard, nil))) }()
	client := http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.Now().Add(3 * time.Second)
	for {
		response, err := client.Get("http://" + address + "/health/live")
		if err == nil {
			_ = response.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("HTTP never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("HTTP failed to stop")
	}
	if checker.Ready(context.Background()).Status != "unavailable" {
		t.Fatal("readiness not disabled on stop")
	}
}
