package event

import (
	"sync"
)

type EventType string

const (
	AgentRegistered   EventType = "AgentRegistered"
	AgentConnected    EventType = "AgentConnected"
	AgentDisconnected EventType = "AgentDisconnected"
	AgentOnline       EventType = "AgentOnline"
	AgentOffline      EventType = "AgentOffline"
	VPNConnected      EventType = "VPNConnected"
	VPNDisconnected   EventType = "VPNDisconnected"
	ProxyCreated      EventType = "ProxyCreated"
	ProxyDeleted      EventType = "ProxyDeleted"
	ProxyFailed       EventType = "ProxyFailed"
	HealthChanged     EventType = "HealthChanged"
	CommandCreated    EventType = "CommandCreated"
	CommandDispatched EventType = "CommandDispatched"
	CommandExecuting  EventType = "CommandExecuting"
	CommandExecuted   EventType = "CommandExecuted"
)

type Event struct {
	Type     EventType   `json:"type"`
	AgentID  string      `json:"agent_id"`
	TargetID string      `json:"target_id,omitempty"` // VPNNodeID or ProxyID
	Payload  interface{} `json:"payload,omitempty"`
}

type Listener func(Event)

type EventBus struct {
	mu        sync.RWMutex
	listeners map[EventType][]Listener
}

var (
	globalBus *EventBus
	once      sync.Once
)

func GetBus() *EventBus {
	once.Do(func() {
		globalBus = &EventBus{
			listeners: make(map[EventType][]Listener),
		}
	})
	return globalBus
}

func (b *EventBus) Subscribe(t EventType, l Listener) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners[t] = append(b.listeners[t], l)
}

func (b *EventBus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	listeners := b.listeners[e.Type]
	for _, l := range listeners {
		go l(e) // Invoke asynchronously to avoid blocking publishers
	}
}
