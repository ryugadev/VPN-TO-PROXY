package vpn

import (
	"context"
	"errors"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
)

type MockExpressVPNDriver struct {
	mu     sync.Mutex
	active map[string]*MockVpnInterface
}

func NewMockExpressVPNDriver() *MockExpressVPNDriver {
	return &MockExpressVPNDriver{
		active: make(map[string]*MockVpnInterface),
	}
}

func (d *MockExpressVPNDriver) Connect(ctx context.Context, node *domain.VPNNode) (domain.VpnInterface, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if inf, ok := d.active[node.ID]; ok {
		return inf, nil
	}

	// Simulated connection latency
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(200 * time.Millisecond):
	}

	// Map IP and country based on location alias
	ip := "103.10.10.10"
	country := "Vietnam"
	region := "Asia Pacific"
	isp := "Viettel Group"
	asn := "AS7552"

	switch node.LocationAlias {
	case "vietnam":
		ip = "103.10.10.10"
		country = "Vietnam"
		region = "Asia Pacific"
		isp = "Viettel Group"
		asn = "AS7552"
	case "singapore":
		ip = "45.90.90.10"
		country = "Singapore"
		region = "Asia Pacific"
		isp = "M247"
		asn = "AS9009"
	case "malaysia":
		ip = "175.139.0.1"
		country = "Malaysia"
		region = "Asia Pacific"
		isp = "TM Net"
		asn = "AS4788"
	case "japan-tokyo":
		ip = "150.95.10.10"
		country = "Japan"
		region = "Asia Pacific"
		isp = "GMO Internet"
		asn = "AS3791"
	case "usa-new-york":
		ip = "198.51.100.10"
		country = "United States"
		region = "Americas"
		isp = "DigitalOcean"
		asn = "AS14061"
	case "usa-los-angeles":
		ip = "198.51.100.20"
		country = "United States"
		region = "Americas"
		isp = "DigitalOcean"
		asn = "AS14061"
	case "uk-london":
		ip = "82.102.23.109"
		country = "United Kingdom"
		region = "Europe"
		isp = "NordVPN"
		asn = "AS13678"
	case "germany-frankfurt":
		ip = "185.190.140.23"
		country = "Germany"
		region = "Europe"
		isp = "Mullvad VPN"
		asn = "AS20001"
	}

	node.IP = ip
	node.PublicIP = ip
	node.Country = country
	node.DetectedCountry = country
	node.Region = region
	node.SelectedCountry = country
	node.SelectedRegion = region
	node.ISP = isp
	node.ASN = asn
	node.LatencyMs = 45
	node.Status = "connected"
	node.ConnectionStatus = "connected"
	node.InterfaceName = "mock-expressvpn0"
	node.AssignedInterface = "mock-expressvpn0"
	now := time.Now()
	node.LastConnectedAt = &now

	inf := &MockVpnInterface{
		id:            node.ID,
		name:          node.Name,
		localIP:       "10.200.0.99",
		interfaceName: "mock-expressvpn0",
		status:        "connected",
	}

	d.active[node.ID] = inf
	return inf, nil
}

func (d *MockExpressVPNDriver) Disconnect(ctx context.Context, node *domain.VPNNode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.active, node.ID)
	node.Status = "disconnected"
	node.ConnectionStatus = "disconnected"
	node.InterfaceName = ""
	node.AssignedInterface = ""
	node.IP = ""
	node.PublicIP = ""
	return nil
}

func (d *MockExpressVPNDriver) ListLocations(ctx context.Context) ([]VPNLocation, error) {
	return FallbackLocations, nil
}

func (d *MockExpressVPNDriver) ValidateCredentials(ctx context.Context, credential *domain.VPNCredential) error {
	if credential.EncryptedSecret == "" && credential.MaskedSecret == "" {
		return errors.New("empty activation code")
	}
	return nil
}

func (d *MockExpressVPNDriver) Activate(ctx context.Context, credential *domain.VPNCredential) error {
	// always succeeds in mock
	return nil
}
