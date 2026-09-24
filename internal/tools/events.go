package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/haramj/openstack-mcp-server/internal/openstack"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"sync"
	"time"
)

const eventsURI = "openstack://events"

type observedEvent struct {
	Sequence   uint64 `json:"sequence"`
	At         string `json:"at"`
	InstanceID string `json:"instance_id,omitempty"`
	Before     string `json:"before,omitempty"`
	After      string `json:"after,omitempty"`
	Kind       string `json:"kind"`
}
type EventFeed struct {
	mu            sync.Mutex
	subscriptions map[*mcp.ServerSession]bool
	events        []observedEvent
	previous      map[string]string
	sequence      uint64
	available     bool
	observed      bool
	lastPoll      string
	read          func(context.Context) ([]openstack.Instance, error)
}

func NewEventFeed() *EventFeed {
	return &EventFeed{subscriptions: map[*mcp.ServerSession]bool{}, read: openstack.ListInstances}
}
func (f *EventFeed) Subscribe(ctx context.Context, r *mcp.SubscribeRequest) error {
	if r.Params.URI != eventsURI {
		return fmt.Errorf("only openstack://events supports subscriptions")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.subscriptions) >= 100 && !f.subscriptions[r.Session] {
		return fmt.Errorf("subscription limit reached")
	}
	f.subscriptions[r.Session] = true
	return nil
}
func (f *EventFeed) Unsubscribe(ctx context.Context, r *mcp.UnsubscribeRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.subscriptions, r.Session)
	return nil
}
func (f *EventFeed) Register(server *mcp.Server) {
	server.AddResource(&mcp.Resource{URI: eventsURI, Name: "Observed instance transitions", MIMEType: "application/json", Description: "Bounded in-memory history from 15-second polling while subscribed. Not an OpenStack notification bus."}, func(ctx context.Context, r *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		data, _ := json.Marshal(map[string]any{"events": f.events, "available": f.available, "observed": f.observed, "latest_sequence": f.sequence, "retained_events": 100, "poll_interval_seconds": 15, "last_poll": f.lastPoll, "active_subscriptions": len(f.subscriptions), "coverage": "polling may miss transitions; history resets on restart; sequence gaps require a fresh snapshot"})
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: eventsURI, MIMEType: "application/json", Text: string(data)}}}, nil
	})
}
func (f *EventFeed) poll(ctx context.Context) bool {
	instances, err := f.read(ctx)
	if len(instances) > 2000 {
		err = errors.New("inventory exceeds observation bound")
	}
	for _, instance := range instances {
		if len(instance.ID) > 256 || len(instance.Status) > 128 {
			err = errors.New("instance fields exceed observation bound")
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastPoll = time.Now().UTC().Format(time.RFC3339Nano)
	changed := false
	add := func(event observedEvent) {
		f.sequence++
		event.Sequence = f.sequence
		event.At = time.Now().UTC().Format(time.RFC3339Nano)
		f.events = append(f.events, event)
		if len(f.events) > 100 {
			f.events = f.events[len(f.events)-100:]
		}
		changed = true
	}
	if err != nil {
		if !f.observed || f.available {
			add(observedEvent{Kind: "coverage_unavailable"})
		}
		f.available = false
		f.observed = true
		return changed
	}
	current := map[string]string{}
	for _, instance := range instances {
		current[instance.ID] = instance.Status
	}
	if !f.observed || !f.available {
		add(observedEvent{Kind: "baseline"})
	} else {
		for id, status := range current {
			if before, ok := f.previous[id]; !ok {
				add(observedEvent{Kind: "appeared", InstanceID: id, After: status})
			} else if before != status {
				add(observedEvent{Kind: "status_changed", InstanceID: id, Before: before, After: status})
			}
		}
		for id, status := range f.previous {
			if _, ok := current[id]; !ok {
				add(observedEvent{Kind: "no_longer_observed", InstanceID: id, Before: status})
			}
		}
	}
	f.previous = current
	f.available = true
	f.observed = true
	return changed
}
func (f *EventFeed) Run(ctx context.Context, server *mcp.Server) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			active := map[*mcp.ServerSession]bool{}
			for session := range server.Sessions() {
				active[session] = true
			}
			f.mu.Lock()
			for session := range f.subscriptions {
				if !active[session] {
					delete(f.subscriptions, session)
				}
			}
			hasSubscribers := len(f.subscriptions) > 0
			if !hasSubscribers {
				f.available = false
			}
			f.mu.Unlock()
			if hasSubscribers && f.poll(ctx) {
				notifyCtx, cancel := context.WithTimeout(ctx, time.Second)
				_ = server.ResourceUpdated(notifyCtx, &mcp.ResourceUpdatedNotificationParams{URI: eventsURI})
				cancel()
			}
		}
	}
}
