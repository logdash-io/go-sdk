package logdash

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/go-retryablehttp"
)

type (
	// TrackOption configures a single event sent with [Logdash.Track].
	TrackOption func(*eventEntry)

	// eventEntry represents a single event to be sent to the server.
	eventEntry struct {
		Name   string                     `json:"name"`
		Props  map[string]json.RawMessage `json:"props,omitempty"`
		UserID string                     `json:"userId,omitempty"`
		ID     string                     `json:"id,omitempty"`
		App    string                     `json:"app,omitempty"`
	}
)

var (
	// eventNamePattern is the event name format the server accepts.
	eventNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

	// websiteEvents belong to the website tracker, so the server rejects them here.
	websiteEvents = []string{"pageview", "pageleave", "browser_error"}
)

// WithUserID sets the internal ID of the signed-in user who triggered the event, never an email address.
//
// Logdash stores only a hash of it, the same one the website tracker's identify sends,
// so a user counts as one person across the website and every app.
// Without it, Logdash counts a visitor of that app that changes every day.
func WithUserID(id string) TrackOption {
	return func(e *eventEntry) {
		e.UserID = id
	}
}

func (ld *Logdash) setupEvents(o *options) {
	if o.apiKey == "" {
		ld.internalLogger.Warn("No API key provided, printing events to the console only")
		return
	}

	ld.internalLogger.VerboseF("Creating events with host %s", o.host)
	client := newHTTPClient(o, ld.internalLogger)
	// 429 means the hourly event limit is reached, which retrying within seconds does not fix
	client.client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			return false, nil
		}
		return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
	}

	ld.events = newAsyncProcessor(
		o.bufferSize,
		func(entry eventEntry) error {
			entry.App = o.app
			return client.sendData("/events", http.MethodPost, entry)
		},
		func(entry eventEntry, err error) {
			ld.internalLogger.ErrorF("Failed to send event %s: %v", entry.Name, err)
		},
	)
	ld.events.SetOverflowPolicy(o.overflowPolicy)
}

// Track sends the event name with props to the server in the background, without blocking
// unless [OverflowPolicyBlock] is set.
//
// Events show up in the domain's Analytics as goals.
// The name must match ^[a-z][a-z0-9_]{0,63}$ and must not be pageview, pageleave or browser_error,
// which belong to the website tracker.
// Props are up to 10 string, number or boolean values, never personal data.
// Track drops an event with an invalid name, or a user ID that contains @, and warns about it in verbose mode.
//
// Without an API key, Track prints the event to the console only.
// [Logdash.Shutdown] waits for the queued events to be sent.
func (ld *Logdash) Track(name string, props map[string]any, opts ...TrackOption) {
	entry := eventEntry{Name: name}
	for _, opt := range opts {
		opt(&entry)
	}

	if !eventNamePattern.MatchString(name) || slices.Contains(websiteEvents, name) {
		ld.internalLogger.WarnF("Dropping event %q: its name must match %s and not be pageview, pageleave or browser_error", name, eventNamePattern)
		return
	}
	if strings.Contains(entry.UserID, "@") {
		ld.internalLogger.WarnF("Dropping event %q: the user ID must be an internal ID, never an email address", name)
		return
	}

	// encoded now, so the caller can change the map while the event waits to be sent
	entry.Props = make(map[string]json.RawMessage, len(props))
	for key, value := range props {
		raw, err := json.Marshal(value)
		if err != nil {
			ld.internalLogger.WarnF("Dropping prop %q of event %q: %v", key, name, err)
			continue
		}
		entry.Props[key] = raw
	}

	if ld.events == nil {
		body, _ := json.Marshal(entry) // never fails: every field is a string or already JSON
		ld.Logger.Info("Event", string(body))
		return
	}

	// a retry sends the same body, so the server stores the event once
	entry.ID = newEventID()
	ld.events.send(entry)
}

// newEventID returns a random UUID v4, or "" to leave the id out when the system has no randomness.
func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = b[6]&0x0f | 0x40 // version 4
	b[8] = b[8]&0x3f | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
