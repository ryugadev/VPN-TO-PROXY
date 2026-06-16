package vpn

import (
	"context"
	"fmt"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"

	"github.com/google/uuid"
)

type VpnManager struct {
	mu          sync.RWMutex
	drivers     map[string]domain.VpnDriver
	active      map[string]domain.VpnInterface
	nodeRepo    domain.VPNNodeRepository
	auditRepo   domain.AuditLogRepository
	credRepo    domain.VPNCredentialRepository
	agentRepo   domain.AgentRepository
	sessionRepo domain.ExpressVPNSessionRepository
}

func NewVpnManager(
	nodeRepo domain.VPNNodeRepository,
	auditRepo domain.AuditLogRepository,
	credRepo domain.VPNCredentialRepository,
	agentRepo domain.AgentRepository,
	sessionRepo domain.ExpressVPNSessionRepository,
) *VpnManager {
	mgr := &VpnManager{
		drivers:     make(map[string]domain.VpnDriver),
		active:      make(map[string]domain.VpnInterface),
		nodeRepo:    nodeRepo,
		auditRepo:   auditRepo,
		credRepo:    credRepo,
		agentRepo:   agentRepo,
		sessionRepo: sessionRepo,
	}

	// Register drivers
	mgr.RegisterDriver("mock", NewMockVpnDriver())
	mgr.RegisterDriver("wireguard", NewWireGuardDriver())
	mgr.RegisterDriver("expressvpn", NewExpressVPNDriver(credRepo, sessionRepo))
	mgr.RegisterDriver("expressvpn_mock", NewMockExpressVPNDriver())

	return mgr
}

func (m *VpnManager) RegisterDriver(driverType string, driver domain.VpnDriver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.drivers[driverType] = driver
}

func (m *VpnManager) Connect(ctx context.Context, nodeID string) (domain.VpnInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	node, err := m.nodeRepo.GetByID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to find vpn node: %v", err)
	}

	if node.Status == "connected" {
		if activeInf, ok := m.active[node.ID]; ok {
			return activeInf, nil
		}
	}

	// ExpressVPN host-level routing single-session check
	if node.Type == "expressvpn" || node.Type == "expressvpn_mock" {
		activeSession, err := m.sessionRepo.GetActiveByAgent(node.AgentID)
		if err == nil && activeSession != nil && activeSession.NodeID != node.ID {
			return nil, fmt.Errorf("Agent already has an active ExpressVPN session. Disconnect it first or use another agent.")
		}
	}

	driver, exists := m.drivers[node.Type]
	if !exists {
		return nil, fmt.Errorf("unsupported vpn type: %s", node.Type)
	}

	node.Status = "connecting"
	node.ConnectionStatus = "connecting"
	m.nodeRepo.Update(node)

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "VPN_CONNECTING",
		Details:   fmt.Sprintf("Connecting to VPN: %s (%s)", node.Name, node.Type),
		Timestamp: time.Now(),
	})

	inf, err := driver.Connect(ctx, node)
	if err != nil {
		node.Status = "failed"
		node.ConnectionStatus = "failed"
		node.LastError = err.Error()
		m.nodeRepo.Update(node)
		m.auditRepo.Create(&domain.AuditLog{
			Action:    "VPN_CONNECT_FAILED",
			Details:   fmt.Sprintf("Failed to connect to VPN %s: %v", node.Name, err),
			Timestamp: time.Now(),
		})
		return nil, err
	}

	// If ExpressVPN connection succeeded, save session data
	if node.Type == "expressvpn" || node.Type == "expressvpn_mock" {
		m.sessionRepo.Create(&domain.ExpressVPNSession{
			ID:                  uuid.New().String(),
			AgentID:             node.AgentID,
			NodeID:              node.ID,
			LocationAlias:       node.LocationAlias,
			LocationDisplayName: node.LocationDisplayName,
			PublicIP:            node.IP,
			DetectedCountry:     node.DetectedCountry,
			Status:              "connected",
			ConnectedAt:         time.Now(),
		})
	}

	m.active[node.ID] = inf
	m.nodeRepo.Update(node)

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "VPN_CONNECTED",
		Details:   fmt.Sprintf("Successfully connected to VPN: %s, exit IP: %s", node.Name, node.IP),
		Timestamp: time.Now(),
	})

	return inf, nil
}

func (m *VpnManager) Disconnect(ctx context.Context, nodeID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node, err := m.nodeRepo.GetByID(nodeID)
	if err != nil {
		return fmt.Errorf("failed to find vpn node: %v", err)
	}

	driver, exists := m.drivers[node.Type]
	if !exists {
		return fmt.Errorf("unsupported vpn type: %s", node.Type)
	}

	err = driver.Disconnect(ctx, node)
	if err != nil {
		m.auditRepo.Create(&domain.AuditLog{
			Action:    "VPN_DISCONNECT_FAILED",
			Details:   fmt.Sprintf("Failed to disconnect VPN %s: %v", node.Name, err),
			Timestamp: time.Now(),
		})
		return err
	}

	// Update active ExpressVPN session status to disconnected
	if node.Type == "expressvpn" || node.Type == "expressvpn_mock" {
		activeSession, err := m.sessionRepo.GetActiveByAgent(node.AgentID)
		if err == nil && activeSession != nil {
			activeSession.Status = "disconnected"
			now := time.Now()
			activeSession.DisconnectedAt = &now
			m.sessionRepo.Update(activeSession)
		}
	}

	delete(m.active, node.ID)
	m.nodeRepo.Update(node)

	m.auditRepo.Create(&domain.AuditLog{
		Action:    "VPN_DISCONNECTED",
		Details:   fmt.Sprintf("Disconnected from VPN: %s", node.Name),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *VpnManager) GetActiveInterface(nodeID string) (domain.VpnInterface, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inf, ok := m.active[nodeID]
	return inf, ok
}

func (m *VpnManager) ListLocations(ctx context.Context, driverType string) ([]VPNLocation, error) {
	m.mu.RLock()
	drv, exists := m.drivers[driverType]
	m.mu.RUnlock()
	if !exists {
		return nil, fmt.Errorf("driver not found: %s", driverType)
	}

	type locationLister interface {
		ListLocations(ctx context.Context) ([]VPNLocation, error)
	}
	if lister, ok := drv.(locationLister); ok {
		return lister.ListLocations(ctx)
	}
	return FallbackLocations, nil
}
