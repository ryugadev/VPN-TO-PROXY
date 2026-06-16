package cache

import (
	"sync"
	"time"
	"vpn-to-proxy/internal/vpn"
)

type LocationCache struct {
	mu         sync.RWMutex
	locations  []vpn.VPNLocation
	expiration time.Time
	ttl        time.Duration
}

func NewLocationCache(ttl time.Duration) *LocationCache {
	return &LocationCache{
		ttl: ttl,
	}
}

func (c *LocationCache) Get() ([]vpn.VPNLocation, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.locations == nil || time.Now().After(c.expiration) {
		return nil, false
	}
	return c.locations, true
}

func (c *LocationCache) Set(locations []vpn.VPNLocation) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.locations = locations
	c.expiration = time.Now().Add(c.ttl)
}

func (c *LocationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.locations = nil
}
