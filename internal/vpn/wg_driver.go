package vpn

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"vpn-to-proxy/internal/domain"
)

type WgInterface struct {
	id            string
	name          string
	localIP       string
	interfaceName string
	status        string
	namespace     string
}

func (w *WgInterface) GetID() string            { return w.id }
func (w *WgInterface) GetName() string          { return w.name }
func (w *WgInterface) GetLocalIP() string       { return w.localIP }
func (w *WgInterface) GetInterfaceName() string { return w.interfaceName }
func (w *WgInterface) GetStatus() string        { return w.status }
func (w *WgInterface) GetNamespace() string    { return w.namespace }

type WireGuardDriver struct {
	mu     sync.Mutex
	active map[string]*WgInterface
}

func NewWireGuardDriver() *WireGuardDriver {
	return &WireGuardDriver{
		active: make(map[string]*WgInterface),
	}
}

func (d *WireGuardDriver) Connect(ctx context.Context, node *domain.VPNNode) (domain.VpnInterface, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("WireGuard driver with Network Namespaces is only supported on Linux")
	}

	if inf, ok := d.active[node.ID]; ok {
		return inf, nil
	}

	nsName := fmt.Sprintf("ns-%s", node.ID[:8])
	ifaceName := fmt.Sprintf("wg-%s", node.ID[:8])

	// Create temp file for configuration
	tmpFile, err := ioutil.TempFile("", "wg-*.conf")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(node.ConfigText); err != nil {
		return nil, fmt.Errorf("failed to write config file: %v", err)
	}
	tmpFile.Close()

	// Extract local IP from config text to assign to interface
	localIP := parseLocalIP(node.ConfigText)
	if localIP == "" {
		localIP = "10.0.0.2/24" // fallback
	}

	// Execution commands for setting up NetNS and WG
	commands := [][]string{
		{"ip", "netns", "add", nsName},
		{"ip", "link", "add", ifaceName, "type", "wireguard"},
		{"ip", "link", "set", ifaceName, "netns", nsName},
		{"ip", "netns", "exec", nsName, "wg", "setconf", ifaceName, tmpFile.Name()},
		{"ip", "netns", "exec", nsName, "ip", "address", "add", localIP, "dev", ifaceName},
		{"ip", "netns", "exec", nsName, "ip", "link", "set", ifaceName, "up"},
		{"ip", "netns", "exec", nsName, "ip", "link", "set", "lo", "up"},
		{"ip", "netns", "exec", nsName, "ip", "route", "add", "default", "dev", ifaceName},
	}

	for _, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		if out, err := cmd.CombinedOutput(); err != nil {
			// clean up namespace if failed
			exec.Command("ip", "netns", "delete", nsName).Run()
			return nil, fmt.Errorf("failed running %v: %v, output: %s", cmdArgs, err, string(out))
		}
	}

	inf := &WgInterface{
		id:            node.ID,
		name:          node.Name,
		localIP:       localIP,
		interfaceName: ifaceName,
		status:        "connected",
		namespace:     nsName,
	}

	d.active[node.ID] = inf

	node.Status = "connected"
	node.InterfaceName = ifaceName
	node.LocalIP = localIP

	return inf, nil
}

func (d *WireGuardDriver) Disconnect(ctx context.Context, node *domain.VPNNode) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	inf, ok := d.active[node.ID]
	if !ok {
		return nil
	}

	nsName := inf.namespace

	// Delete network namespace (will automatically delete interfaces inside it)
	cmd := exec.Command("ip", "netns", "delete", nsName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to delete netns %s: %v, output: %s", nsName, err, string(out))
	}

	delete(d.active, node.ID)
	node.Status = "disconnected"
	node.InterfaceName = ""
	node.LocalIP = ""

	return nil
}

func parseLocalIP(config string) string {
	lines := strings.Split(config, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "address") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				// handle multi IP or trailing comments
				val = strings.Split(val, ",")[0]
				val = strings.Split(val, "#")[0]
				return strings.TrimSpace(val)
			}
		}
	}
	return ""
}
