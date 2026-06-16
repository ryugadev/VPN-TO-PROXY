package vpn

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/network"
	"vpn-to-proxy/internal/security"
)

type ExpressVPNDriver struct {
	mu          sync.Mutex
	credRepo    domain.VPNCredentialRepository
	sessionRepo domain.ExpressVPNSessionRepository
	active      map[string]*ExpressVpnInf
}

type ExpressVpnInf struct {
	id            string
	name          string
	localIP       string
	interfaceName string
	status        string
}

func (e *ExpressVpnInf) GetID() string            { return e.id }
func (e *ExpressVpnInf) GetName() string          { return e.name }
func (e *ExpressVpnInf) GetLocalIP() string       { return e.localIP }
func (e *ExpressVpnInf) GetInterfaceName() string { return e.interfaceName }
func (e *ExpressVpnInf) GetStatus() string        { return e.status }

func NewExpressVPNDriver(credRepo domain.VPNCredentialRepository, sessionRepo domain.ExpressVPNSessionRepository) *ExpressVPNDriver {
	return &ExpressVPNDriver{
		credRepo:    credRepo,
		sessionRepo: sessionRepo,
		active:      make(map[string]*ExpressVpnInf),
	}
}

// Connect implements domain.VpnDriver
func (d *ExpressVPNDriver) Connect(ctx context.Context, node *domain.VPNNode) (domain.VpnInterface, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. Verify expressvpn CLI exists
	if _, err := exec.LookPath("expressvpn"); err != nil {
		return nil, errors.New("expressvpn CLI binary not found in system path")
	}

	// 2. Fetch credential
	cred, err := d.credRepo.GetByID(node.CredentialID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VPN credential: %v", err)
	}

	// 3. Decrypt credential secret
	secret, err := security.DecryptSecret(cred.EncryptedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credential secret: %v", err)
	}

	// 4. Check whether ExpressVPN is activated
	if !d.isActivated(ctx) {
		cmd := exec.CommandContext(ctx, "expressvpn", "activate", secret)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("failed to activate ExpressVPN: %v, output: %s", err, string(out))
		}
	}

	// 5. Disable diagnostics
	exec.CommandContext(ctx, "expressvpn", "preferences", "set", "send_diagnostics", "false").Run()

	// 6. Set protocol
	protocol := "auto"
	if node.Protocol != "" {
		protocol = node.Protocol
	}
	exec.CommandContext(ctx, "expressvpn", "protocol", protocol).Run()

	// 7. Connect selected location
	cmdConnect := exec.CommandContext(ctx, "expressvpn", "connect", node.LocationAlias)
	if out, err := cmdConnect.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to connect ExpressVPN to %s: %v, output: %s", node.LocationAlias, err, string(out))
	}

	// 8. Poll status until connected (max 15s)
	connected := false
	for i := 0; i < 15; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(1 * time.Second):
		}

		statusCmd := exec.CommandContext(ctx, "expressvpn", "status")
		out, err := statusCmd.CombinedOutput()
		if err == nil && strings.Contains(strings.ToLower(string(out)), "connected to") {
			connected = true
			break
		}
	}

	if !connected {
		exec.CommandContext(ctx, "expressvpn", "disconnect").Run()
		return nil, errors.New("timeout waiting for ExpressVPN connection to establish")
	}

	// 9. Resolve public IP and location details
	publicIP, err := network.ResolvePublicIP(ctx)
	if err != nil {
		// Try one more time
		publicIP, err = network.ResolvePublicIP(ctx)
		if err != nil {
			publicIP = "0.0.0.0" // fallback
		}
	}

	geo, err := network.ResolveGeoIP(ctx, publicIP)
	detectedCountry := "Unknown"
	if err == nil && geo != nil {
		detectedCountry = geo.Country
		node.ISP = geo.ISP
		node.ASN = geo.AS
	}

	// Save session details
	node.IP = publicIP
	node.PublicIP = publicIP
	node.DetectedCountry = detectedCountry
	node.Status = "connected"
	node.ConnectionStatus = "connected"
	node.AssignedInterface = "tun0" // default for expressvpn
	node.InterfaceName = "tun0"
	now := time.Now()
	node.LastConnectedAt = &now
	node.LastError = ""

	inf := &ExpressVpnInf{
		id:            node.ID,
		name:          node.Name,
		localIP:       "10.0.0.1", // expressvpn virtual interface
		interfaceName: "tun0",
		status:        "connected",
	}

	d.active[node.ID] = inf
	return inf, nil
}

// Disconnect implements domain.VpnDriver
func (d *ExpressVPNDriver) Disconnect(ctx context.Context, node *domain.VPNNode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	cmd := exec.CommandContext(ctx, "expressvpn", "disconnect")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to disconnect ExpressVPN: %v, output: %s", err, string(out))
	}

	delete(d.active, node.ID)
	node.Status = "disconnected"
	node.ConnectionStatus = "disconnected"
	node.IP = ""
	node.PublicIP = ""
	node.InterfaceName = ""
	node.AssignedInterface = ""

	return nil
}

func (d *ExpressVPNDriver) ListLocations(ctx context.Context) ([]VPNLocation, error) {
	if _, err := exec.LookPath("expressvpn"); err != nil {
		return FallbackLocations, nil
	}

	cmd := exec.CommandContext(ctx, "expressvpn", "list", "all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return FallbackLocations, nil
	}

	return ParseExpressVpnLocations(string(out)), nil
}

func (d *ExpressVPNDriver) isActivated(ctx context.Context) bool {
	cmd := exec.CommandContext(ctx, "expressvpn", "status")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	outStr := string(out)
	if strings.Contains(strings.ToLower(outStr), "not activated") || strings.Contains(strings.ToLower(outStr), "activate") {
		return false
	}
	return true
}
