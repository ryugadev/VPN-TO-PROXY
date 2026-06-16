package vpn

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"
	"vpn-to-proxy/internal/domain"
)

type MockVpnInterface struct {
	id            string
	name          string
	localIP       string
	interfaceName string
	status        string
}

func (m *MockVpnInterface) GetID() string            { return m.id }
func (m *MockVpnInterface) GetName() string          { return m.name }
func (m *MockVpnInterface) GetLocalIP() string       { return m.localIP }
func (m *MockVpnInterface) GetInterfaceName() string { return m.interfaceName }
func (m *MockVpnInterface) GetStatus() string        { return m.status }

type MockVpnDriver struct {
	mu      sync.Mutex
	active  map[string]*MockVpnInterface
	counter int
}

func NewMockVpnDriver() *MockVpnDriver {
	return &MockVpnDriver{
		active: make(map[string]*MockVpnInterface),
	}
}

func (d *MockVpnDriver) Connect(ctx context.Context, node *domain.VPNNode) (domain.VpnInterface, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if inf, ok := d.active[node.ID]; ok {
		return inf, nil
	}

	d.counter++
	interfaceName := fmt.Sprintf("mock-wg%d", d.counter)
	localIP := fmt.Sprintf("10.200.0.%d", d.counter+1)

	// Choose random mock geolocations
	geos := []struct {
		IP      string
		Country string
		Region  string
		ISP     string
		ASN     string
	}{
		{"104.244.72.11", "United States", "California", "Twitter Inc.", "AS13414"},
		{"185.190.140.23", "Germany", "Frankfurt", "Mullvad VPN", "AS20001"},
		{"82.102.23.109", "United Kingdom", "London", "NordVPN", "AS13678"},
		{"210.140.10.45", "Japan", "Tokyo", "Softbank", "AS2519"},
		{"103.84.22.1", "Vietnam", "Hanoi", "Viettel Group", "AS7552"},
	}

	geo := geos[rand.Intn(len(geos))]
	node.IP = geo.IP
	node.Country = geo.Country
	node.Region = geo.Region
	node.ISP = geo.ISP
	node.ASN = geo.ASN
	node.LatencyMs = int64(20 + rand.Intn(80))

	inf := &MockVpnInterface{
		id:            node.ID,
		name:          node.Name,
		localIP:       localIP,
		interfaceName: interfaceName,
		status:        "connected",
	}

	d.active[node.ID] = inf

	// update node status
	node.Status = "connected"
	node.InterfaceName = interfaceName
	node.LocalIP = localIP
	now := time.Now()
	node.LastConnectedAt = &now

	return inf, nil
}

func (d *MockVpnDriver) Disconnect(ctx context.Context, node *domain.VPNNode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.active, node.ID)
	node.Status = "disconnected"
	node.InterfaceName = ""
	node.LocalIP = ""

	return nil
}
