package fluxer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeGateway is a tiny gateway server: Hello → wait for Identify →
// Ready + one dispatch event → see if a heartbeat arrives.
type fakeGateway struct {
	connections chan *websocket.Conn
	hello       map[string]any
}

func (f *fakeGateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("encoding") != "json" {
		http.Error(w, "want ?v=1&encoding=json", http.StatusBadRequest)
		return
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	conn.WriteJSON(f.hello)
	f.connections <- conn
}

func newFakeGateway(t *testing.T) (*Gateway, *fakeGateway) {
	t.Helper()
	fake := &fakeGateway{
		connections: make(chan *websocket.Conn, 4),
		hello: map[string]any{
			"op": opHello,
			"d":  map[string]any{"heartbeat_interval": 100},
		},
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	return NewGateway(url, "test-token"), fake
}

// readUntil reads messages from conn until pred matches, or fails after 2s.
func readUntil(t *testing.T, conn *websocket.Conn, what string, pred func(m message) bool) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %s: %v", what, err)
		}
		var msg message
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		if pred(msg) {
			return
		}
	}
}

func TestGatewayIdentifiesThenSendsEvents(t *testing.T) {
	gateway, fake := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		gateway.Run(ctx)
		close(done)
	}()

	// First connection: we must receive an Identify with the token.
	conn := <-fake.connections
	readUntil(t, conn, "identify", func(m message) bool {
		if m.Op != opIdentify {
			return false
		}
		var payload struct {
			Token string `json:"token"`
		}
		json.Unmarshal(m.D, &payload)
		if payload.Token != "test-token" {
			t.Errorf("identify token = %q, want test-token", payload.Token)
		}
		return true
	})

	// Server sends READY, then a GUILD_MEMBER_ADD dispatch.
	conn.WriteJSON(map[string]any{
		"op": opDispatch, "t": "READY", "s": 1,
		"d": map[string]any{"session_id": "abc"},
	})
	conn.WriteJSON(map[string]any{
		"op": opDispatch, "t": "GUILD_MEMBER_ADD", "s": 2,
		"d": map[string]any{"guild_id": "999"},
	})

	wantNames := []string{"READY", "GUILD_MEMBER_ADD"}
	for _, want := range wantNames {
		select {
		case ev := <-gateway.Events:
			if ev.Name != want {
				t.Errorf("event = %q, want %q", ev.Name, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no event %q within 2s", want)
		}
	}

	// Heartbeat must arrive (interval was 40ms).
	readUntil(t, conn, "heartbeat", func(m message) bool {
		return m.Op == opHeartbeat
	})

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestGatewaySendsSequenceInHeartbeat(t *testing.T) {
	gateway, fake := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go gateway.Run(ctx)

	conn := <-fake.connections
	readUntil(t, conn, "identify", func(m message) bool { return m.Op == opIdentify })

	// After dispatch with s=42, the next heartbeat must carry d=42.
	conn.WriteJSON(map[string]any{"op": opDispatch, "t": "T", "s": 42, "d": nil})
	readUntil(t, conn, "heartbeat with seq 42", func(m message) bool {
		if m.Op != opHeartbeat {
			return false
		}
		var d any
		json.Unmarshal(m.D, &d)
		if d != float64(42) {
			t.Errorf("heartbeat d = %v, want 42", d)
			return false
		}
		return true
	})

	cancel()
}

func TestGatewayReconnectsAfterConnectionDrop(t *testing.T) {
	gateway, fake := newFakeGateway(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		gateway.Run(ctx)
		close(done)
	}()

	// First connection: close it from the server side.
	conn := <-fake.connections
	readUntil(t, conn, "identify", func(m message) bool { return m.Op == opIdentify })
	conn.Close()

	// Run must dial again. Backoff starts at 1s, so allow 3s.
	select {
	case conn2 := <-fake.connections:
		readUntil(t, conn2, "second identify", func(m message) bool { return m.Op == opIdentify })
		conn2.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("no reconnect within 3s")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

func TestDiscoveryParsesRealShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/fluxer" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"endpoints":{"api_public":"https://api.example","gateway":"wss://gw.example"}}`))
	}))
	defer server.Close()

	disco, err := Discover(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if disco.Endpoints.APIPublic != "https://api.example" {
		t.Errorf("api_public = %q", disco.Endpoints.APIPublic)
	}
	if disco.Endpoints.Gateway != "wss://gw.example" {
		t.Errorf("gateway = %q", disco.Endpoints.Gateway)
	}
}

func TestDiscoveryRejectsMissingEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"endpoints":{"gateway":"wss://gw.example"}}`))
	}))
	defer server.Close()

	if _, err := Discover(context.Background(), server.URL); err == nil {
		t.Fatal("want error when api_public is missing, got nil")
	}
}

func TestDiscoveryRejectsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	if _, err := Discover(context.Background(), server.URL); err == nil {
		t.Fatal("want error on HTTP 503, got nil")
	}
}
