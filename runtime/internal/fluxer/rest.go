// Package fluxer talks to a Fluxer instance: REST (this file) and gateway (later).
//
// Everything a module does over HTTP goes through this Client, so rate limits,
// errors and dry-run behave the same everywhere.
package fluxer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is a REST client for one Fluxer instance.
type Client struct {
	// BaseURL is the API root from /.well-known/fluxer, e.g. "https://fluxer.app".
	BaseURL string

	// Token is the bot token, without any prefix. Sent as: Authorization: Bot <token>.
	Token string

	// DryRun logs non-GET requests instead of performing them.
	DryRun bool

	// mutex makes requests happen one at a time: Fluxer's rate limits are
	// global per bot, so there is no point firing requests in parallel.
	mutex sync.Mutex

	// sleepUntil is set when the server tells us to slow down. The next
	// request waits until then before it is sent.
	sleepUntil time.Time

	http *http.Client
}

// NewClient returns a client for the given instance URL and bot token.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// APIError is a non-2xx response. It names the method, path and status so the
// log alone tells you what failed.
type APIError struct {
	Method string
	Path   string
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// Do sends one request.
//
//	method  "GET", "POST", "PUT", "DELETE"
//	path    e.g. "/channels/123/messages"
//	body    marshalled to JSON and sent, or nil for no body
//	out     JSON response is unmarshalled here, or nil to ignore it
func (c *Client) Do(method, path string, body any, out any) error {
	if c.DryRun && method != http.MethodGet {
		slog.Info("dry-run: skipping request", "method", method, "path", path)
		return nil
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Encode the body once; we may send it twice (429 retry).
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: encode body: %w", method, path, err)
		}
	}

	// Two attempts: the second one only happens after a 429.
	for attempt := 0; attempt < 2; attempt++ {
		if wait := time.Until(c.sleepUntil); wait > 0 {
			slog.Info("rate limited, waiting", "wait", wait.Round(time.Millisecond))
			time.Sleep(wait)
		}

		respBody, header, err := c.send(method, path, payload)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}

		if respBody.status == http.StatusTooManyRequests && attempt == 0 {
			c.sleepUntil = time.Now().Add(parseRetryAfter(header))
			continue
		}

		if respBody.status < 200 || respBody.status >= 300 {
			return &APIError{
				Method: method,
				Path:   path,
				Status: respBody.status,
				Body:   strings.TrimSpace(respBody.text),
			}
		}

		if out != nil && len(respBody.text) > 0 {
			if err := json.Unmarshal([]byte(respBody.text), out); err != nil {
				return fmt.Errorf("%s %s: decode response: %w", method, path, err)
			}
		}
		return nil
	}
	return nil
}

type httpResult struct {
	status int
	text   string
}

// send performs a single HTTP round trip and returns the status plus body text.
func (c *Client) send(method, path string, payload []byte) (httpResult, http.Header, error) {
	url := c.BaseURL + path

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return httpResult{}, nil, err
	}
	req.Header.Set("Authorization", "Bot "+c.Token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return httpResult{}, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return httpResult{}, nil, fmt.Errorf("read body: %w", err)
	}

	// Remember when the bucket refills, so the next request waits if empty.
	c.noteRateLimit(resp.Header, method)

	return httpResult{status: resp.StatusCode, text: string(body)}, resp.Header, nil
}

// noteRateLimit reads the standard rate-limit headers. If the bucket is empty
// (remaining = 0), the next request waits until it refills.
func (c *Client) noteRateLimit(header http.Header, method string) {
	remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	if err != nil || remaining > 0 {
		return
	}
	resetAfter, err := strconv.ParseFloat(header.Get("X-RateLimit-Reset-After"), 64)
	if err != nil {
		return
	}
	c.sleepUntil = time.Now().Add(time.Duration(resetAfter * float64(time.Second)))
	slog.Info("rate limit bucket empty", "method", method, "reset_after", resetAfter)
}

// parseRetryAfter reads how long to wait after a 429. The header is seconds;
// anything unparseable defaults to 1s.
func parseRetryAfter(header http.Header) time.Duration {
	seconds, err := strconv.ParseFloat(header.Get("Retry-After"), 64)
	if err != nil {
		return time.Second
	}
	return time.Duration(seconds * float64(time.Second))
}

// User is the bot's own identity.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Bot      bool   `json:"bot"`
}

// GetCurrentUser asks the instance who we are. It proves the token works.
func (c *Client) GetCurrentUser() (*User, error) {
	var user User
	if err := c.Do(http.MethodGet, "/applications/@me", nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateMessage posts a text message to a channel.
func (c *Client) CreateMessage(channelID, content string) error {
	body := map[string]string{"content": content}
	return c.Do(http.MethodPost, "/channels/"+channelID+"/messages", body, nil)
}

// CreateDM opens (or reopens) a direct message with a user and returns the
// channel ID messages should be posted to.
func (c *Client) CreateDM(userID string) (string, error) {
	body := map[string]string{"recipient_id": userID}
	var channel struct {
		ID string `json:"id"`
	}
	if err := c.Do(http.MethodPost, "/users/@me/channels", body, &channel); err != nil {
		return "", err
	}
	if channel.ID == "" {
		return "", fmt.Errorf("POST /users/@me/channels: response has no channel id")
	}
	return channel.ID, nil
}

// AddMemberRole gives a member a role (Manage Roles permission, role below the bot's).
func (c *Client) AddMemberRole(guildID, userID, roleID string) error {
	path := "/guilds/" + guildID + "/members/" + userID + "/roles/" + roleID
	return c.Do(http.MethodPut, path, nil, nil)
}

// RemoveMemberRole takes a role away from a member.
func (c *Client) RemoveMemberRole(guildID, userID, roleID string) error {
	path := "/guilds/" + guildID + "/members/" + userID + "/roles/" + roleID
	return c.Do(http.MethodDelete, path, nil, nil)
}
