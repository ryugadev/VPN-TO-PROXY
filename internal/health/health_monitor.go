package health

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/event"

	"golang.org/x/net/proxy"
)

type HealthMonitorService struct {
	mu           sync.RWMutex
	proxyRepo    domain.ProxyRepository
	vpnRepo      domain.VPNNodeRepository
	agentRepo    domain.AgentRepository
	metricRepo   domain.HealthMetricRepository
	auditRepo    domain.AuditLogRepository
	stopChan     chan struct{}
	currentGrade string // A, B, C, Dead
	latencyMs    int64
	eventBus     *event.EventBus
}

func NewHealthMonitorService(
	proxyRepo domain.ProxyRepository,
	vpnRepo domain.VPNNodeRepository,
	agentRepo domain.AgentRepository,
	metricRepo domain.HealthMetricRepository,
	auditRepo domain.AuditLogRepository,
) *HealthMonitorService {
	return &HealthMonitorService{
		proxyRepo:    proxyRepo,
		vpnRepo:      vpnRepo,
		agentRepo:    agentRepo,
		metricRepo:   metricRepo,
		auditRepo:    auditRepo,
		stopChan:     make(chan struct{}),
		currentGrade: "A",
		eventBus:     event.GetBus(),
	}
}

func (s *HealthMonitorService) Start(ctx context.Context) {
	go s.runCheckLoop()
}

func (s *HealthMonitorService) Stop() {
	close(s.stopChan)
}

func (s *HealthMonitorService) GetCurrentGrade() (string, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentGrade, s.latencyMs
}

func (s *HealthMonitorService) runCheckLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Initial check
	s.check()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.check()
		}
	}
}

func (s *HealthMonitorService) check() {
	proxies, err := s.proxyRepo.List()
	if err != nil {
		return
	}

	agents, err := s.agentRepo.List()
	if err != nil {
		return
	}

	var totalLatency int64
	var checkCount int
	var offlineProxies int
	var warningAgents int
	var offlineAgents int

	// 1. Check Agents
	for _, agent := range agents {
		if agent.Status == "offline" {
			offlineAgents++
		} else if agent.Status == "warning" {
			warningAgents++
		}
	}

	// 2. Check Proxies and VPNs
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, prxy := range proxies {
		if prxy.Status != "running" {
			continue
		}

		wg.Add(1)
		go func(p domain.Proxy) {
			defer wg.Done()
			lat, ok := s.checkProxyNode(p)

			mu.Lock()
			if ok {
				totalLatency += lat
				checkCount++
			} else {
				offlineProxies++
			}
			mu.Unlock()
		}(prxy)
	}

	wg.Wait()

	// Overall latency calculation
	var avgLatency int64
	if checkCount > 0 {
		avgLatency = totalLatency / int64(checkCount)
	}

	// 3. Assign Health Grades
	grade := "A"
	if offlineAgents > 0 || offlineProxies > 0 {
		grade = "C"
	} else if warningAgents > 0 || avgLatency > 200 {
		grade = "B"
	}

	// If everything is down or main local agent is offline, it is Dead
	isLocalAgentOffline := false
	for _, agent := range agents {
		if agent.ID == "local-agent" && agent.Status == "offline" {
			isLocalAgentOffline = true
		}
	}

	if isLocalAgentOffline || (len(proxies) > 0 && offlineProxies == len(proxies)) {
		grade = "Dead"
	}

	s.mu.Lock()
	prevGrade := s.currentGrade
	s.currentGrade = grade
	s.latencyMs = avgLatency
	s.mu.Unlock()

	if prevGrade != grade {
		s.eventBus.Publish(event.Event{
			Type:    event.HealthChanged,
			Payload: grade,
		})
		s.auditRepo.Create(&domain.AuditLog{
			Action:    "HEALTH_GRADE_CHANGED",
			Details:   fmt.Sprintf("Global health grade transitioned from %s to %s", prevGrade, grade),
			Timestamp: time.Now(),
		})
	}
}

func (s *HealthMonitorService) checkProxyNode(prxy domain.Proxy) (int64, bool) {
	startTime := time.Now()
	proxyAddr := fmt.Sprintf("127.0.0.1:%d", prxy.Port)

	var transport *http.Transport

	if prxy.Type == "socks5" {
		var auth *proxy.Auth
		if prxy.Username != "" && prxy.Password != "" {
			auth = &proxy.Auth{
				User:     prxy.Username,
				Password: prxy.Password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			return 0, false
		}

		transport = &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
	} else {
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
		if err != nil {
			return 0, false
		}
		if prxy.Username != "" && prxy.Password != "" {
			proxyURL.User = url.UserPassword(prxy.Username, prxy.Password)
		}
		transport = &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   7 * time.Second,
	}

	// Fetch simple response to check latency and exit
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return 0, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, false
	}

	latency := time.Since(startTime).Milliseconds()

	// Write health metric to DB
	s.metricRepo.Create(&domain.HealthMetric{
		TargetID:   prxy.ID,
		TargetType: "proxy",
		LatencyMs:  latency,
		Status:     "online",
		CheckedAt:  time.Now(),
	})

	return latency, true
}
