package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	gopsnet "github.com/shirou/gopsutil/v3/net"
	"golang.org/x/net/websocket"
)

type Config struct {
	AgentID                  string `yaml:"agent_id"`
	MasterURL                string `yaml:"master_url"`
	Token                    string `yaml:"token"`
	HeartbeatIntervalSeconds int    `yaml:"heartbeat_interval_seconds"`
	MetricsIntervalSeconds   int    `yaml:"metrics_interval_seconds"`
	ProxyPortRangeStart      int    `yaml:"proxy_port_range_start"`
	ProxyPortRangeEnd        int    `yaml:"proxy_port_range_end"`
}

type CommandMessage struct {
	Type           string          `json:"type"`
	CommandID      string          `json:"command_id"`
	CommandType    string          `json:"command_type"`
	Payload        json.RawMessage `json:"payload"`
	TimeoutSeconds int             `json:"timeout_seconds"`
	IdempotencyKey string          `json:"idempotency_key"`
}

type Agent struct {
	cfg       Config
	ws        *websocket.Conn
	vpnMu     sync.Mutex
	proxyMu   sync.Mutex
	proxies   map[int]net.Listener
	startedAt time.Time
}

func main() {
	configPath := flag.String("config", "cmd/agent/agent.yaml", "Path to vpn-agent YAML config")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	a := &Agent{cfg: cfg, proxies: make(map[int]net.Listener), startedAt: time.Now()}
	a.run(context.Background())
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.AgentID == "" || cfg.MasterURL == "" || cfg.Token == "" || cfg.Token == "YOUR_AGENT_TOKEN" {
		return Config{}, errors.New("agent_id, master_url, and token are required")
	}
	if cfg.HeartbeatIntervalSeconds == 0 {
		cfg.HeartbeatIntervalSeconds = 30
	}
	if cfg.MetricsIntervalSeconds == 0 {
		cfg.MetricsIntervalSeconds = 5
	}
	if cfg.ProxyPortRangeStart == 0 {
		cfg.ProxyPortRangeStart = 8000
	}
	if cfg.ProxyPortRangeEnd == 0 {
		cfg.ProxyPortRangeEnd = 9000
	}
	return cfg, nil
}

func (a *Agent) run(ctx context.Context) {
	for {
		if err := a.connectAndServe(ctx); err != nil {
			log.Printf("agent disconnected: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (a *Agent) connectAndServe(ctx context.Context) error {
	wsURL, err := url.Parse(a.cfg.MasterURL)
	if err != nil {
		return err
	}
	origin := "http://" + wsURL.Host
	cfg, err := websocket.NewConfig(a.cfg.MasterURL, origin)
	if err != nil {
		return err
	}
	cfg.Header = http.Header{"Authorization": []string{"Bearer " + a.cfg.Token}}

	ws, err := websocket.DialConfig(cfg)
	if err != nil {
		return err
	}
	a.ws = ws
	defer ws.Close()

	log.Printf("vpn-agent %s connected to %s", a.cfg.AgentID, a.cfg.MasterURL)
	a.sendStateSync()

	done := make(chan error, 1)
	go a.telemetryLoop(ctx, done)
	go a.readLoop(done)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (a *Agent) telemetryLoop(ctx context.Context, done chan<- error) {
	hbTicker := time.NewTicker(time.Duration(a.cfg.HeartbeatIntervalSeconds) * time.Second)
	metricsTicker := time.NewTicker(time.Duration(a.cfg.MetricsIntervalSeconds) * time.Second)
	defer hbTicker.Stop()
	defer metricsTicker.Stop()

	_ = a.sendHeartbeat()
	_ = a.sendMetrics()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hbTicker.C:
			if err := a.sendHeartbeat(); err != nil {
				done <- err
				return
			}
		case <-metricsTicker.C:
			if err := a.sendMetrics(); err != nil {
				done <- err
				return
			}
		}
	}
}

func (a *Agent) readLoop(done chan<- error) {
	for {
		var msg string
		if err := websocket.Message.Receive(a.ws, &msg); err != nil {
			done <- err
			return
		}
		var cmd CommandMessage
		if err := json.Unmarshal([]byte(msg), &cmd); err != nil || cmd.Type != "command" {
			continue
		}
		go a.executeCommand(cmd)
	}
}

func (a *Agent) executeCommand(cmd CommandMessage) {
	_ = a.sendCommandResult(cmd.CommandID, "executing", "", "")

	ctx := context.Background()
	if cmd.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cmd.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	var result string
	var err error
	switch cmd.CommandType {
	case "CONNECT_VPN", "DISCONNECT_VPN", "HEALTH_CHECK", "SYNC_STATE":
		a.vpnMu.Lock()
		result, err = a.executeVPNCommand(ctx, cmd)
		a.vpnMu.Unlock()
	case "CREATE_PROXY":
		result, err = a.createProxy(cmd.Payload)
	case "DELETE_PROXY":
		result, err = a.deleteProxy(cmd.Payload)
	case "RESTART_PROXY":
		result, err = a.restartProxy(cmd.Payload)
	default:
		err = fmt.Errorf("unsupported command type %s", cmd.CommandType)
	}

	if err != nil {
		_ = a.sendCommandResult(cmd.CommandID, "failed", "", err.Error())
		return
	}
	_ = a.sendCommandResult(cmd.CommandID, "completed", result, "")
}

func (a *Agent) executeVPNCommand(ctx context.Context, cmd CommandMessage) (string, error) {
	switch cmd.CommandType {
	case "CONNECT_VPN":
		var payload struct {
			Location string `json:"location"`
		}
		_ = json.Unmarshal(cmd.Payload, &payload)
		args := []string{"connect"}
		if payload.Location != "" {
			args = append(args, payload.Location)
		}
		return runExpressVPN(ctx, args...)
	case "DISCONNECT_VPN":
		return runExpressVPN(ctx, "disconnect")
	case "HEALTH_CHECK":
		return runExpressVPN(ctx, "status")
	case "SYNC_STATE":
		a.sendStateSync()
		return `{"synced":true}`, nil
	default:
		return "", fmt.Errorf("unsupported vpn command")
	}
}

func runExpressVPN(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("expressvpn"); err != nil {
		return fmt.Sprintf(`{"driver":"expressvpn","mock":true,"args":%q}`, args), nil
	}
	out, err := exec.CommandContext(ctx, "expressvpn", args...).CombinedOutput()
	return string(out), err
}

func (a *Agent) createProxy(payload json.RawMessage) (string, error) {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	var input struct {
		Port int `json:"port"`
	}
	_ = json.Unmarshal(payload, &input)
	port := input.Port
	if port == 0 {
		port = a.nextFreePortLocked()
	}
	if port == 0 {
		return "", errors.New("no free proxy port available")
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return "", err
	}
	a.proxies[port] = ln
	go servePlaceholderProxy(ln)
	return fmt.Sprintf(`{"port":%d,"status":"running"}`, port), nil
}

func (a *Agent) deleteProxy(payload json.RawMessage) (string, error) {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()

	port, err := portFromPayload(payload)
	if err != nil {
		return "", err
	}
	ln, ok := a.proxies[port]
	if !ok {
		return "", fmt.Errorf("proxy port %d is not running", port)
	}
	_ = ln.Close()
	delete(a.proxies, port)
	return fmt.Sprintf(`{"port":%d,"status":"stopped"}`, port), nil
}

func (a *Agent) restartProxy(payload json.RawMessage) (string, error) {
	_, _ = a.deleteProxy(payload)
	return a.createProxy(payload)
}

func portFromPayload(payload json.RawMessage) (int, error) {
	var input struct {
		Port int `json:"port"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return 0, err
	}
	if input.Port == 0 {
		return 0, errors.New("port is required")
	}
	return input.Port, nil
}

func (a *Agent) nextFreePortLocked() int {
	for port := a.cfg.ProxyPortRangeStart; port <= a.cfg.ProxyPortRangeEnd; port++ {
		if _, used := a.proxies[port]; used {
			continue
		}
		return port
	}
	return 0
}

func servePlaceholderProxy(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"))
		}(conn)
	}
}

func (a *Agent) sendHeartbeat() error {
	cpuUsage, ramUsage, _, _, vpnCount, proxyCount := a.sampleMetrics()
	return a.sendJSON(map[string]interface{}{
		"type":        "heartbeat",
		"cpu_usage":   cpuUsage,
		"ram_usage":   ramUsage,
		"vpn_count":   vpnCount,
		"proxy_count": proxyCount,
	})
}

func (a *Agent) sendMetrics() error {
	cpuUsage, ramUsage, diskUsage, netIn, vpnCount, proxyCount := a.sampleMetrics()
	return a.sendJSON(map[string]interface{}{
		"type":        "metrics",
		"cpu_usage":   cpuUsage,
		"ram_usage":   ramUsage,
		"disk_usage":  diskUsage,
		"net_in":      netIn,
		"net_out":     uint64(0),
		"vpn_count":   vpnCount,
		"proxy_count": proxyCount,
	})
}

func (a *Agent) sendStateSync() {
	info, _ := host.Info()
	public := ""
	_ = a.sendJSON(map[string]interface{}{
		"type":           "state_sync",
		"agent_id":       a.cfg.AgentID,
		"hostname":       info.Hostname,
		"os":             runtime.GOOS,
		"agent_version":  "v0.2.0-phase2b",
		"active_vpn":     nil,
		"active_proxies": a.proxyPorts(),
		"public_ip":      public,
		"started_at":     a.startedAt.Format(time.RFC3339),
	})
}

func (a *Agent) sendCommandResult(commandID, status, result, lastError string) error {
	return a.sendJSON(map[string]interface{}{
		"type":        "command_result",
		"command_id":  commandID,
		"status":      status,
		"result":      result,
		"last_error":  lastError,
		"reported_at": time.Now().Format(time.RFC3339),
	})
}

func (a *Agent) sendJSON(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return websocket.Message.Send(a.ws, string(b))
}

func (a *Agent) sampleMetrics() (float64, float64, float64, uint64, int, int) {
	cpuUsage := 0.0
	if vals, err := cpu.Percent(0, false); err == nil && len(vals) > 0 {
		cpuUsage = vals[0]
	}
	ramUsage := 0.0
	if vm, err := mem.VirtualMemory(); err == nil {
		ramUsage = vm.UsedPercent
	}
	diskUsage := 0.0
	if du, err := disk.Usage("."); err == nil {
		diskUsage = du.UsedPercent
	}
	var netIn uint64
	if counters, err := gopsnet.IOCounters(false); err == nil && len(counters) > 0 {
		netIn = counters[0].BytesRecv
	}
	return cpuUsage, ramUsage, diskUsage, netIn, 0, len(a.proxyPorts())
}

func (a *Agent) proxyPorts() []int {
	a.proxyMu.Lock()
	defer a.proxyMu.Unlock()
	ports := make([]int, 0, len(a.proxies))
	for port := range a.proxies {
		ports = append(ports, port)
	}
	return ports
}
