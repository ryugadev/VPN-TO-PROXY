package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
)

type ProxyManager struct {
	mu         sync.Mutex
	servers    map[int]*GoProxyServer
	proxyRepo  domain.ProxyRepository
	nodeRepo   domain.VPNNodeRepository
	auditRepo  domain.AuditLogRepository
	vpnManager VpnInterfaceProvider
}

// Interface provider to prevent import loop between proxy and vpn
type VpnInterfaceProvider interface {
	GetActiveInterface(nodeID string) (domain.VpnInterface, bool)
}

func NewProxyManager(
	proxyRepo domain.ProxyRepository,
	nodeRepo domain.VPNNodeRepository,
	auditRepo domain.AuditLogRepository,
	vpnManager VpnInterfaceProvider,
) *ProxyManager {
	return &ProxyManager{
		servers:    make(map[int]*GoProxyServer),
		proxyRepo:  proxyRepo,
		nodeRepo:   nodeRepo,
		auditRepo:  auditRepo,
		vpnManager: vpnManager,
	}
}

func (m *ProxyManager) StartProxy(proxyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prxy, err := m.proxyRepo.GetByID(proxyID)
	if err != nil {
		return fmt.Errorf("proxy not found: %v", err)
	}

	if _, exists := m.servers[prxy.Port]; exists {
		prxy.Status = "running"
		m.proxyRepo.Update(prxy)
		return nil
	}

	if prxy.ExpiresAt != nil && time.Now().After(*prxy.ExpiresAt) {
		now := time.Now()
		prxy.Status = "expired"
		prxy.RevokedAt = &now
		m.proxyRepo.Update(prxy)
		m.auditRepo.Create(&domain.AuditLog{
			Action:    "PROXY_EXPIRED",
			Details:   fmt.Sprintf("Proxy on port %d expired before start", prxy.Port),
			Timestamp: now,
		})
		return fmt.Errorf("proxy on port %d has expired", prxy.Port)
	}

	// Fetch outbound IP from connected VPN (if set)
	var outboundIP string
	var netnsName string
	if prxy.VPNNodeID != "" {
		if vpnIface, ok := m.vpnManager.GetActiveInterface(prxy.VPNNodeID); ok {
			// If the VPN lives in a Linux network namespace (WireGuard driver),
			// dial through that namespace so traffic egresses via the tunnel.
			// Otherwise fall back to binding the tunnel's local IP as the
			// outbound source address.
			if nsProvider, ok := vpnIface.(interface{ GetNamespace() string }); ok && nsProvider.GetNamespace() != "" {
				netnsName = nsProvider.GetNamespace()
			} else {
				outboundIP = vpnIface.GetLocalIP()
			}
		} else {
			// If VPN isn't connected, we can still start the proxy, but outbound traffic routes via standard default network interface.
			// Or we can choose to reject if sticky VPN binding is required. For Phase 1, we start it but log a warning.
			m.auditRepo.Create(&domain.AuditLog{
				Action:    "PROXY_WARNING",
				Details:   fmt.Sprintf("Starting Proxy on port %d with unbound VPN (VPN Node %s not active)", prxy.Port, prxy.VPNNodeID),
				Timestamp: time.Now(),
			})
		}
	}

	server := NewAuthenticatedGoProxyServer(prxy.ID, prxy.Port, prxy.BindIP, outboundIP, prxy.Username, prxy.Password)
	if netnsName != "" {
		server.SetNamespace(netnsName)
	}
	if err := server.Start(); err != nil {
		prxy.Status = "error"
		m.proxyRepo.Update(prxy)
		m.auditRepo.Create(&domain.AuditLog{
			Action:    "PROXY_START_FAILED",
			Details:   fmt.Sprintf("Failed to start Proxy on port %d: %v", prxy.Port, err),
			Timestamp: time.Now(),
		})
		return err
	}

	m.servers[prxy.Port] = server
	prxy.Status = "running"
	m.proxyRepo.Update(prxy)

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "PROXY_STARTED",
		Details:   fmt.Sprintf("Proxy started on port %d (%s), VPN: %s", prxy.Port, prxy.Type, prxy.VPNNodeID),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *ProxyManager) StopProxy(proxyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prxy, err := m.proxyRepo.GetByID(proxyID)
	if err != nil {
		return fmt.Errorf("proxy not found: %v", err)
	}

	server, exists := m.servers[prxy.Port]
	if exists {
		server.Stop()
		delete(m.servers, prxy.Port)
	}

	prxy.Status = "stopped"
	m.proxyRepo.Update(prxy)

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "PROXY_STOPPED",
		Details:   fmt.Sprintf("Proxy stopped on port %d", prxy.Port),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *ProxyManager) ExpireProxy(proxyID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	prxy, err := m.proxyRepo.GetByID(proxyID)
	if err != nil {
		return fmt.Errorf("proxy not found: %v", err)
	}

	server, exists := m.servers[prxy.Port]
	if exists {
		server.Stop()
		delete(m.servers, prxy.Port)
	}

	now := time.Now()
	prxy.Status = "expired"
	prxy.RevokedAt = &now
	if err := m.proxyRepo.Update(prxy); err != nil {
		return err
	}

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "PROXY_EXPIRED",
		Details:   fmt.Sprintf("Proxy expired on port %d", prxy.Port),
		Timestamp: now,
	})

	return nil
}

func (m *ProxyManager) SweepExpiredProxies() {
	proxies, err := m.proxyRepo.List()
	if err != nil {
		return
	}
	now := time.Now()
	for _, prxy := range proxies {
		if prxy.ExpiresAt != nil && now.After(*prxy.ExpiresAt) && prxy.Status == "running" {
			_ = m.ExpireProxy(prxy.ID)
		} else if prxy.ExpiresAt != nil && now.After(*prxy.ExpiresAt) && prxy.Status == "stopped" {
			prxy.Status = "expired"
			prxy.RevokedAt = &now
			_ = m.proxyRepo.Update(&prxy)
		}
	}
}

func (m *ProxyManager) StartExpiryMonitor(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.SweepExpiredProxies()
			}
		}
	}()
}

func (m *ProxyManager) RestoreProxies() {
	proxies, err := m.proxyRepo.List()
	if err != nil {
		return
	}

	for _, prxy := range proxies {
		if prxy.ExpiresAt != nil && time.Now().After(*prxy.ExpiresAt) {
			now := time.Now()
			prxy.Status = "expired"
			prxy.RevokedAt = &now
			_ = m.proxyRepo.Update(&prxy)
			continue
		}
		if prxy.Status == "running" {
			m.StartProxy(prxy.ID)
		}
	}
}
