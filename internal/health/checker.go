package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
	"vpn-to-proxy/internal/domain"

	"golang.org/x/net/proxy"
)

type HealthChecker struct {
	proxyRepo  domain.ProxyRepository
	nodeRepo   domain.VPNNodeRepository
	metricRepo domain.HealthMetricRepository
	auditRepo  domain.AuditLogRepository
	interval   time.Duration
	stopChan   chan struct{}
}

type IPInfo struct {
	IP      string `json:"ip"`
	Country string `json:"country"`
	Region  string `json:"region"`
	Org     string `json:"org"`
}

func NewHealthChecker(
	proxyRepo domain.ProxyRepository,
	nodeRepo domain.VPNNodeRepository,
	metricRepo domain.HealthMetricRepository,
	auditRepo domain.AuditLogRepository,
	interval time.Duration,
) *HealthChecker {
	return &HealthChecker{
		proxyRepo:  proxyRepo,
		nodeRepo:   nodeRepo,
		metricRepo: metricRepo,
		auditRepo:  auditRepo,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

func (c *HealthChecker) Start() {
	ticker := time.NewTicker(c.interval)
	go func() {
		// Run initial check
		c.CheckAll()

		for {
			select {
			case <-ticker.C:
				c.CheckAll()
			case <-c.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (c *HealthChecker) Stop() {
	close(c.stopChan)
}

func (c *HealthChecker) CheckAll() {
	proxies, err := c.proxyRepo.List()
	if err != nil {
		return
	}

	for _, prxy := range proxies {
		if prxy.Status == "running" {
			go c.CheckProxy(prxy)
		}
	}
}

func (c *HealthChecker) CheckProxy(prxy domain.Proxy) {
	startTime := time.Now()
	var client *http.Client

	proxyAddr := fmt.Sprintf("127.0.0.1:%d", prxy.Port)

	if prxy.Type == "socks5" {
		// Set up SOCKS5 client dialer
		var auth *proxy.Auth
		if prxy.Username != "" && prxy.Password != "" {
			auth = &proxy.Auth{
				User:     prxy.Username,
				Password: prxy.Password,
			}
		}

		dialer, err := proxy.SOCKS5("tcp", proxyAddr, auth, proxy.Direct)
		if err != nil {
			c.handleFailure(prxy, err)
			return
		}

		tbTransport := &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.Dial(network, addr)
			},
		}
		client = &http.Client{
			Transport: tbTransport,
			Timeout:   10 * time.Second,
		}
	} else {
		// HTTP proxy client setup
		proxyURL, err := url.Parse(fmt.Sprintf("http://%s", proxyAddr))
		if err != nil {
			c.handleFailure(prxy, err)
			return
		}

		// Configure basic auth credentials if present
		if prxy.Username != "" && prxy.Password != "" {
			proxyURL.User = url.UserPassword(prxy.Username, prxy.Password)
		}

		tbTransport := &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		}
		client = &http.Client{
			Transport: tbTransport,
			Timeout:   10 * time.Second,
		}
	}

	// Fetch public IP details to verify VPN egress
	resp, err := client.Get("http://ipinfo.io/json")
	if err != nil {
		c.handleFailure(prxy, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.handleFailure(prxy, fmt.Errorf("status code %d", resp.StatusCode))
		return
	}

	var info IPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		c.handleFailure(prxy, err)
		return
	}

	latency := time.Since(startTime).Milliseconds()

	// Update Health Metric
	metric := &domain.HealthMetric{
		TargetID:   prxy.ID,
		TargetType: "proxy",
		LatencyMs:  latency,
		Status:     "online",
		CheckedAt:  time.Now(),
	}
	c.metricRepo.Create(metric)

	// Update VPN node metadata based on proxy exit
	if prxy.VPNNodeID != "" {
		node, err := c.nodeRepo.GetByID(prxy.VPNNodeID)
		if err == nil {
			node.IP = info.IP
			node.Country = info.Country
			node.Region = info.Region
			node.ISP = info.Org
			node.LatencyMs = latency
			c.nodeRepo.Update(node)
		}
	}
}

func (c *HealthChecker) handleFailure(prxy domain.Proxy, err error) {
	// Record failure metric
	metric := &domain.HealthMetric{
		TargetID:   prxy.ID,
		TargetType: "proxy",
		LatencyMs:  0,
		Status:     "offline",
		CheckedAt:  time.Now(),
	}
	c.metricRepo.Create(metric)

	// Log warning
	c.auditRepo.Create(&domain.AuditLog{
		Action:    "PROXY_HEALTH_FAILED",
		Details:   fmt.Sprintf("Health check failed for proxy on port %d: %v", prxy.Port, err),
		Timestamp: time.Now(),
	})
}
