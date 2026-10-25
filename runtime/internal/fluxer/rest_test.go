package fluxer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewClient(server.URL, "test-token"), server
}

func TestCreateMessageSendsJSONWithAuth(t *testing.T) {
	var gotAuth, gotContentType string
	var gotBody map[string]string

	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost || r.URL.Path != "/channels/123/messages" {
			t.Errorf("got %s %s, want POST /channels/123/messages", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
	})

	if err := client.CreateMessage("123", "hello world"); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if gotAuth != "Bot test-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bot test-token")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody["content"] != "hello world" {
		t.Errorf("content = %q, want %q", gotBody["content"], "hello world")
	}
}

func TestGetCurrentUserDecodesResponse(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id": "42", "username": "my-bot", "bot": true}`))
	})

	user, err := client.GetCurrentUser()
	if err != nil {
		t.Fatalf("GetCurrentUser: %v", err)
	}
	if user.ID != "42" || user.Username != "my-bot" || !user.Bot {
		t.Errorf("user = %+v, want id=42 username=my-bot bot=true", user)
	}
}

func TestAPIErrorNamesMethodPathAndStatus(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message": "Unknown Channel"}`))
	})

	err := client.CreateMessage("123", "hi")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	want := `POST /channels/123/messages: HTTP 404: {"message": "Unknown Channel"}`
	if err.Error() != want {
		t.Errorf("error = %q\nwant  %q", err.Error(), want)
	}
}

func TestRetriesOnceAfter429(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
	})

	if err := client.CreateMessage("123", "hi"); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("server got %d calls, want 2 (one 429 + one retry)", got)
	}
}

func TestGivesUpAfterSecond429(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	err := client.CreateMessage("123", "hi")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("server got %d calls, want 2 (max)", got)
	}
}

func TestWaitsWhenRateLimitBucketIsEmpty(t *testing.T) {
	var firstCall = true
	var elapsed time.Duration
	var started time.Time
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if firstCall {
			firstCall = false
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset-After", "0.2")
			return
		}
		elapsed = time.Since(started)
	})

	started = time.Now()
	if err := client.CreateMessage("123", "one"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := client.CreateMessage("123", "two"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("second call waited %v, want ~200ms (bucket refill)", elapsed)
	}
}

func TestDryRunSkipsWritesButStillReads(t *testing.T) {
	var calls atomic.Int32
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"id": "42", "username": "my-bot"}`))
	})
	client.DryRun = true

	if err := client.CreateMessage("123", "hi"); err != nil {
		t.Fatalf("CreateMessage in dry-run: %v", err)
	}
	if _, err := client.GetCurrentUser(); err != nil {
		t.Fatalf("GetCurrentUser in dry-run: %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("server got %d calls, want 1 (the GET only)", got)
	}
}

func TestCreateDMSendsRecipientAndReturnsChannelID(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"id": "dm1"}`))
	})

	channelID, err := client.CreateDM("u1")
	if err != nil {
		t.Fatalf("CreateDM: %v", err)
	}
	if gotPath != "/users/@me/channels" {
		t.Errorf("path = %q, want /users/@me/channels", gotPath)
	}
	if gotBody["recipient_id"] != "u1" {
		t.Errorf("recipient_id = %q, want u1", gotBody["recipient_id"])
	}
	if channelID != "dm1" {
		t.Errorf("channel = %q, want dm1", channelID)
	}
}

func TestRemoveMemberRoleUsesDelete(t *testing.T) {
	var gotMethod, gotPath string
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.RemoveMemberRole("g1", "u1", "r1"); err != nil {
		t.Fatalf("RemoveMemberRole: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/guilds/g1/members/u1/roles/r1" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestDeleteMessageUsesDelete(t *testing.T) {
	var gotMethod, gotPath string
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	if err := client.DeleteMessage("c1", "m1"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/channels/c1/messages/m1" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestGetChannelDecodesNameAndGuild(t *testing.T) {
	client, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/channels/c1" {
			t.Errorf("path = %q, want /channels/c1", r.URL.Path)
		}
		w.Write([]byte(`{"id":"c1","guild_id":"g1","name":"general"}`))
	})

	channel, err := client.GetChannel("c1")
	if err != nil {
		t.Fatalf("GetChannel: %v", err)
	}
	if channel.ID != "c1" || channel.GuildID != "g1" || channel.Name != "general" {
		t.Errorf("channel = %+v", channel)
	}
}
