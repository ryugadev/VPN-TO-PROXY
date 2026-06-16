package health

import (
	"context"
	"log"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/event"
)

type CachedHeartbeat struct {
	AgentID    string
	CPUUsage   float64
	RAMUsage   float64
	VPNCount   int
	ProxyCount int
	Timestamp  time.Time
}

type AgentHeartbeatService struct {
	mu           sync.RWMutex
	cache        map[string]*CachedHeartbeat
	agentRepo    domain.AgentRepository
	heartbeatRepo domain.AgentHeartbeatRepository
	eventBus     *event.EventBus
	stopChan     chan struct{}
}

func NewAgentHeartbeatService(agentRepo domain.AgentRepository, heartbeatRepo domain.AgentHeartbeatRepository) *AgentHeartbeatService {
	return &AgentHeartbeatService{
		cache:         make(map[string]*CachedHeartbeat),
		agentRepo:     agentRepo,
		heartbeatRepo: heartbeatRepo,
		eventBus:      event.GetBus(),
		stopChan:      make(chan struct{}),
	}
}

func (s *AgentHeartbeatService) RecordHeartbeat(agentID string, cpuUsage, ramUsage float64, vpnCount, proxyCount int) {
	s.mu.Lock()
	s.cache[agentID] = &CachedHeartbeat{
		AgentID:    agentID,
		CPUUsage:   cpuUsage,
		RAMUsage:   ramUsage,
		VPNCount:   vpnCount,
		ProxyCount: proxyCount,
		Timestamp:  time.Now(),
	}
	s.mu.Unlock()

	// Update Agent in database
	agent, err := s.agentRepo.GetByID(agentID)
	if err == nil && agent != nil {
		prevStatus := agent.Status
		agent.Status = "healthy"
		agent.CPUUsage = cpuUsage
		agent.RAMUsage = ramUsage
		agent.VPNCount = vpnCount
		agent.ProxyCount = proxyCount
		agent.LastHeartbeatAt = time.Now()
		agent.UpdatedAt = time.Now()
		s.agentRepo.Update(agent)

		if prevStatus != "healthy" {
			s.eventBus.Publish(event.Event{
				Type:    event.AgentOnline,
				AgentID: agentID,
			})
		}
	}
}

func (s *AgentHeartbeatService) GetCached(agentID string) (*CachedHeartbeat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hb, ok := s.cache[agentID]
	return hb, ok
}

func (s *AgentHeartbeatService) Start(ctx context.Context) {
	go s.runOfflineSweep()
	go s.runHistoricalAggregator()
}

func (s *AgentHeartbeatService) Stop() {
	close(s.stopChan)
}

func (s *AgentHeartbeatService) runOfflineSweep() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			agents, err := s.agentRepo.List()
			if err != nil {
				continue
			}

			s.mu.RLock()
			now := time.Now()
			for _, agent := range agents {
				// We don't sweep local-agent if it is running in-process, but we still apply standard check
				hb, ok := s.cache[agent.ID]
				lastHeartbeat := agent.LastHeartbeatAt
				if ok {
					lastHeartbeat = hb.Timestamp
				}

				if agent.Status != "offline" && now.Sub(lastHeartbeat) > 90*time.Second {
					s.mu.RUnlock()
					// Mark Offline
					agent.Status = "offline"
					agent.UpdatedAt = time.Now()
					s.agentRepo.Update(&agent)
					s.eventBus.Publish(event.Event{
						Type:    event.AgentOffline,
						AgentID: agent.ID,
					})
					log.Printf("[AgentHeartbeatService] Agent %s is offline (no heartbeat for > 90s)", agent.ID)
					s.mu.RLock()
				}
			}
			s.mu.RUnlock()
		}
	}
}

func (s *AgentHeartbeatService) runHistoricalAggregator() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.mu.RLock()
			for agentID, hb := range s.cache {
				s.heartbeatRepo.Create(&domain.AgentHeartbeat{
					AgentID:    agentID,
					CPUUsage:   hb.CPUUsage,
					RAMUsage:   hb.RAMUsage,
					VPNCount:   hb.VPNCount,
					ProxyCount: hb.ProxyCount,
					Timestamp:  time.Now(),
				})
			}
			s.mu.RUnlock()
			log.Println("[AgentHeartbeatService] Historical heartbeat snapshots written to database")
		}
	}
}
