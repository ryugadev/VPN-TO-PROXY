package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"
	"time"

	"golang.org/x/net/websocket"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/event"
	"vpn-to-proxy/internal/health"
)

type DashboardWSGateway struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]bool
}

func NewDashboardWSGateway(eventBus *event.EventBus) *DashboardWSGateway {
	gw := &DashboardWSGateway{
		clients: make(map[*websocket.Conn]bool),
	}

	eventTypes := []event.EventType{
		event.AgentRegistered,
		event.AgentConnected,
		event.AgentDisconnected,
		event.AgentOnline,
		event.AgentOffline,
		event.VPNConnected,
		event.VPNDisconnected,
		event.ProxyCreated,
		event.ProxyDeleted,
		event.ProxyFailed,
		event.HealthChanged,
		event.CommandCreated,
		event.CommandDispatched,
		event.CommandExecuting,
		event.CommandExecuted,
	}

	for _, et := range eventTypes {
		eventBus.Subscribe(et, func(e event.Event) {
			eBytes, err := json.Marshal(e)
			if err == nil {
				gw.Broadcast(string(eBytes))
			}
		})
	}

	return gw
}

func (gw *DashboardWSGateway) Handler() websocket.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		gw.mu.Lock()
		gw.clients[ws] = true
		gw.mu.Unlock()

		defer func() {
			gw.mu.Lock()
			delete(gw.clients, ws)
			gw.mu.Unlock()
			ws.Close()
		}()

		buf := make([]byte, 512)
		for {
			ws.SetReadDeadline(time.Now().Add(60 * time.Second))
			n, err := ws.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				_ = websocket.Message.Send(ws, "pong")
			}
		}
	})
}

func (gw *DashboardWSGateway) Broadcast(msg string) {
	gw.mu.Lock()
	defer gw.mu.Unlock()

	for client := range gw.clients {
		err := websocket.Message.Send(client, msg)
		if err != nil {
			client.Close()
			delete(gw.clients, client)
		}
	}
}

type AgentWSGateway struct {
	db               *gorm.DB
	credRepo         domain.AgentCredentialRepository
	agentRepo        domain.AgentRepository
	commandBus       *event.CommandBus
	heartbeatService *health.AgentHeartbeatService
	eventBus         *event.EventBus
}

func NewAgentWSGateway(
	db *gorm.DB,
	credRepo domain.AgentCredentialRepository,
	agentRepo domain.AgentRepository,
	commandBus *event.CommandBus,
	heartbeatService *health.AgentHeartbeatService,
	eventBus *event.EventBus,
) *AgentWSGateway {
	return &AgentWSGateway{
		db:               db,
		credRepo:         credRepo,
		agentRepo:        agentRepo,
		commandBus:       commandBus,
		heartbeatService: heartbeatService,
		eventBus:         eventBus,
	}
}

func (gw *AgentWSGateway) Handler() websocket.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		req := ws.Request()
		token := req.URL.Query().Get("token")
		if token == "" {
			authHeader := req.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				token = authHeader[7:]
			}
		}

		if token == "" {
			log.Println("[AgentWS] Handshake rejected: token not found")
			ws.Close()
			return
		}

		h := sha256.Sum256([]byte(token))
		tokenHash := hex.EncodeToString(h[:])

		var cred domain.AgentCredential
		err := gw.db.Where("token_hash = ? AND (revoked_at IS NULL)", tokenHash).First(&cred).Error
		if err != nil {
			log.Printf("[AgentWS] Handshake rejected: invalid/revoked token hash: %s", tokenHash)
			ws.Close()
			return
		}

		if cred.ExpiresAt.Before(time.Now()) {
			log.Printf("[AgentWS] Handshake rejected: expired token for agent %s", cred.AgentID)
			ws.Close()
			return
		}

		agentID := cred.AgentID
		log.Printf("[AgentWS] Connection established & authenticated for agent: %s", agentID)

		sendChan := make(chan []byte, 100)
		gw.commandBus.RegisterAgentConnection(agentID, sendChan)

		// Mark agent as online
		var agent domain.Agent
		if err := gw.db.Where("id = ?", agentID).First(&agent).Error; err == nil {
			agent.Status = "healthy"
			agent.LastHeartbeatAt = time.Now()
			gw.db.Save(&agent)
		}

		gw.eventBus.Publish(event.Event{
			Type:    event.AgentConnected,
			AgentID: agentID,
		})

		defer func() {
			gw.commandBus.UnregisterAgentConnection(agentID)
			close(sendChan)
			ws.Close()

			if err := gw.db.Where("id = ?", agentID).First(&agent).Error; err == nil {
				agent.Status = "offline"
				gw.db.Save(&agent)
			}

			gw.eventBus.Publish(event.Event{
				Type:    event.AgentDisconnected,
				AgentID: agentID,
			})
		}()

		// Writer goroutine
		go func() {
			for msg := range sendChan {
				err := websocket.Message.Send(ws, string(msg))
				if err != nil {
					log.Printf("[AgentWS] Send error to agent %s: %v", agentID, err)
					return
				}
			}
		}()

		// Read loop
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		for {
			var msgStr string
			err := websocket.Message.Receive(ws, &msgStr)
			if err != nil {
				break
			}

			ws.SetReadDeadline(time.Now().Add(60 * time.Second))

			var rawMsg map[string]interface{}
			if err := json.Unmarshal([]byte(msgStr), &rawMsg); err != nil {
				continue
			}

			msgType, _ := rawMsg["type"].(string)
			switch msgType {
			case "heartbeat":
				cpu, _ := rawMsg["cpu_usage"].(float64)
				ram, _ := rawMsg["ram_usage"].(float64)
				vpnCount, _ := rawMsg["vpn_count"].(float64)
				proxyCount, _ := rawMsg["proxy_count"].(float64)
				gw.heartbeatService.RecordHeartbeat(agentID, cpu, ram, int(vpnCount), int(proxyCount))

			case "metrics":
				cpu, _ := rawMsg["cpu_usage"].(float64)
				ram, _ := rawMsg["ram_usage"].(float64)
				disk, _ := rawMsg["disk_usage"].(float64)
				netIn, _ := rawMsg["net_in"].(float64)
				netOut, _ := rawMsg["net_out"].(float64)
				vpnCount, _ := rawMsg["vpn_count"].(float64)
				proxyCount, _ := rawMsg["proxy_count"].(float64)

				gw.heartbeatService.RecordHeartbeat(agentID, cpu, ram, int(vpnCount), int(proxyCount))

				snapshot := &domain.SystemMetricSnapshot{
					CPUUsage:   cpu,
					RAMUsage:   ram,
					DiskUsage:  disk,
					NetIn:      uint64(netIn),
					NetOut:     uint64(netOut),
					VPNCount:   int(vpnCount),
					ProxyCount: int(proxyCount),
					Timestamp:  time.Now(),
				}
				_ = gw.db.Create(snapshot).Error

			case "command_result":
				cmdID, _ := rawMsg["command_id"].(string)
				status, _ := rawMsg["status"].(string)
				resText, _ := rawMsg["result"].(string)
				errText, _ := rawMsg["last_error"].(string)

				_ = gw.commandBus.HandleResult(cmdID, status, resText, errText)

			case "state_sync":
				log.Printf("[AgentWS] Received State Sync from agent %s", agentID)
			}
		}
	})
}
