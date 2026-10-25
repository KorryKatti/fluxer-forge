package fluxer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Gateway opcodes (part of the Fluxer gateway contract).
const (
	opDispatch        = 0  // a real event; carries name in T and data in D
	opHeartbeat       = 1  // both sides use it: we send one, the server may ask for one
	opIdentify        = 2  // we send this once, right after Hello
	opResume          = 6  // we send this to keep our session after a drop
	opHeartbeatAck    = 11 // server confirms our heartbeat
	opHello           = 10 // server greets us and says how often to heartbeat
	opInvalidSession  = 9
	opReconnect       = 7
	opServerHeartbeat = 6
)

// Event is one gateway event, e.g. Name "READY" or "GUILD_MEMBER_ADD".
type Event struct {
	Name string
	Data json.RawMessage
}

// Gateway keeps one websocket connection to a Fluxer instance alive and
// hands every event to Events. If the connection drops, it reconnects.
type Gateway struct {
	// URL is the gateway endpoint from discovery, e.g. "wss://gateway.fluxer.app".
	URL string

	// Token is the bot token, sent as-is in the Identify payload.
	Token string

	// Events receives every event. Created automatically if nil.
	// Buffered: if nobody reads for a while, events are dropped (and logged).
	Events chan Event

	// Fatal receives a reason when reconnecting cannot help, e.g. a rejected
	// token. Run then stops for good.
	Fatal chan error

	// ready is set once this connection reached READY; Run uses it to reset
	// the reconnect backoff. Only touched by the Run goroutine.
	ready bool

	// sessionID and seq survive a dropped connection so the next socket can
	// Resume instead of starting over. Only touched by the Run goroutine.
	sessionID string
	seq       atomic.Int64
}

func NewGateway(url, token string) *Gateway {
	return &Gateway{
		URL:    url,
		Token:  token,
		Events: make(chan Event, 64),
		Fatal:  make(chan error, 1),
	}
}

// Run keeps the gateway connected until ctx is cancelled. It never returns
// an error: failures are logged and lead to a reconnect instead.
func (g *Gateway) Run(ctx context.Context) {
	backoff := time.Second

	for {
		g.ready = false
		err := g.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if fatal := fatalClose(err); fatal != nil {
			g.Fatal <- fatal
			return
		}
		if shouldResetSession(err) {
			g.sessionID = ""
			g.seq.Store(0)
		}
		if err != nil {
			slog.Warn("gateway disconnected", "error", err, "retry_in", backoff)
		}
		if g.ready {
			backoff = time.Second // the last session worked, start over
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		backoff = time.Duration(math.Min(float64(backoff*2), float64(30*time.Second)))
	}
}

// shouldResetSession says whether the next connection must Identify instead
// of Resume. 4007 means our sequence was rejected; 4009 means the session
// timed out.
func shouldResetSession(err error) bool {
	if errors.Is(err, errInvalidSession) {
		return true
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		return closeErr.Code == 4007 || closeErr.Code == 4009
	}
	return false
}

// fatalClose turns a close code that reconnecting cannot fix into a message
// for the user. Every other error means "try again".
func fatalClose(err error) error {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return nil
	}
	switch closeErr.Code {
	case 4004:
		return fmt.Errorf("the instance rejected the bot token (close 4004) — check FLUXER_TOKEN")
	case 4011:
		return fmt.Errorf("the instance requires sharding, which this bot does not support (close 4011)")
	default:
		return nil
	}
}

// session is exactly one websocket connection: dial → Hello → Identify →
// Ready → events, until something breaks. Run calls it again after that.
func (g *Gateway) session(ctx context.Context) error {
	// A connection-scoped context stops the heartbeat when the session ends.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, g.URL+"?v=1&encoding=json", nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	// ReadMessage blocks, and cancelling the context alone would not wake it.
	// Closing the connection does; the read loop then returns an error.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// The websocket library allows one writer at a time, but both the read
	// loop and the heartbeat goroutine write. This lock serializes them.
	var writeMu sync.Mutex
	write := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	// The heartbeat runs in its own goroutine until this session ends.
	// It only starts once Hello tells us the interval.
	var heartbeatOnce sync.Once
	startHeartbeat := func(interval time.Duration) {
		heartbeatOnce.Do(func() {
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if err := write(heartbeatPayload(g.seq.Load())); err != nil {
							slog.Debug("heartbeat failed", "error", err)
							return
						}
					}
				}
			}()
		})
	}

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		var msg message
		if err := json.Unmarshal(raw, &msg); err != nil {
			slog.Debug("gateway sent unreadable message", "error", err)
			continue
		}

		switch msg.Op {
		case opHello:
			var hello struct {
				HeartbeatInterval int `json:"heartbeat_interval"` // milliseconds
			}
			json.Unmarshal(msg.D, &hello)
			if hello.HeartbeatInterval <= 0 {
				hello.HeartbeatInterval = 41_000
			}
			startHeartbeat(time.Duration(hello.HeartbeatInterval) * time.Millisecond)

			// Resume when we have a live session, otherwise start a new one.
			if g.sessionID != "" && g.seq.Load() > 0 {
				slog.Info("gateway resuming session", "session", g.sessionID, "seq", g.seq.Load())
				if err := write(resumePayload(g.Token, g.sessionID, g.seq.Load())); err != nil {
					return err
				}
			} else {
				g.seq.Store(0)
				if err := write(identifyPayload(g.Token)); err != nil {
					return err
				}
			}

		case opDispatch:
			if msg.S != nil {
				g.seq.Store(int64(*msg.S))
			}
			if msg.T == "READY" {
				var ready struct {
					SessionID string `json:"session_id"`
				}
				json.Unmarshal(msg.D, &ready)
				g.sessionID = ready.SessionID
				g.ready = true
				slog.Info("gateway ready", "session", g.sessionID)
			}
			if msg.T == "RESUMED" {
				g.ready = true
				slog.Info("gateway resumed")
			}
			g.emit(Event{Name: msg.T, Data: msg.D})

		case opHeartbeat:
			// Server asked for an immediate heartbeat.
			if err := write(heartbeatPayload(g.seq.Load())); err != nil {
				return err
			}

		case opHeartbeatAck:
			// Nothing to do.

		case opReconnect:
			return errServerReconnect

		case opInvalidSession:
			return errInvalidSession

		default:
			slog.Debug("ignoring gateway opcode", "op", msg.Op)
		}
	}
}

// emit hands an event to readers. If nobody is reading, drop it rather than
// freeze the connection.
func (g *Gateway) emit(ev Event) {
	select {
	case g.Events <- ev:
	default:
		slog.Warn("event dropped, nobody is reading the Events channel", "event", ev.Name)
	}
}

// message is the envelope every gateway payload uses.
type message struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"` // payload; RawMessage = "parse it later"
	S  *int            `json:"s"` // sequence number; nil until the first dispatch
	T  string          `json:"t"` // event name, e.g. "READY"
}

func identifyPayload(token string) map[string]any {
	return map[string]any{
		"op": opIdentify,
		"d": map[string]any{
			"token": token,
			"properties": map[string]string{
				"os":      runtime.GOOS,
				"browser": "fluxer-forge",
				"device":  "fluxer-forge",
			},
		},
	}
}

func resumePayload(token, sessionID string, seq int64) map[string]any {
	return map[string]any{
		"op": opResume,
		"d": map[string]any{
			"token":      token,
			"session_id": sessionID,
			"seq":        seq,
		},
	}
}

func heartbeatPayload(seq int64) map[string]any {
	if seq == 0 {
		return map[string]any{"op": opHeartbeat, "d": nil} // null until we have one
	}
	return map[string]any{"op": opHeartbeat, "d": seq}
}

var (
	errServerReconnect = &simpleError{"server asked to reconnect"}
	errInvalidSession  = &simpleError{"session invalid"}
)

type simpleError struct{ text string }

func (e *simpleError) Error() string { return e.text }
