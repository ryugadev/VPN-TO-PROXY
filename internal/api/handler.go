package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
	"vpn-to-proxy/internal/abuse"
	"vpn-to-proxy/internal/billing"
	"vpn-to-proxy/internal/cache"
	"vpn-to-proxy/internal/customer"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/event"
	"vpn-to-proxy/internal/health"
	"vpn-to-proxy/internal/proxy"
	"vpn-to-proxy/internal/routing"
	"vpn-to-proxy/internal/security"
	"vpn-to-proxy/internal/vpn"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	nodeRepo          domain.VPNNodeRepository
	proxyRepo         domain.ProxyRepository
	auditRepo         domain.AuditLogRepository
	metricRepo        domain.HealthMetricRepository
	credRepo          domain.VPNCredentialRepository
	agentRepo         domain.AgentRepository
	agentCredRepo     domain.AgentCredentialRepository
	agentCmdRepo      domain.AgentCommandRepository
	sessionRepo       domain.ExpressVPNSessionRepository
	vpnMgr            *vpn.VpnManager
	proxyMgr          *proxy.ProxyManager
	locCache          *cache.LocationCache
	heartbeatService  *health.AgentHeartbeatService
	metricsService    *health.SystemMetricsService
	validationService *vpn.ExpressVPNValidationService
	healthMonitor     *health.HealthMonitorService
	customerService   *customer.Service
	billingService    *billing.Service
	abuseService      *abuse.Service
	routingService    *routing.Service
	dashboardWS       *DashboardWSGateway
	agentWS           *AgentWSGateway
	commandBus        *event.CommandBus
	eventBus          *event.EventBus
}

func NewHandler(
	nodeRepo domain.VPNNodeRepository,
	proxyRepo domain.ProxyRepository,
	auditRepo domain.AuditLogRepository,
	metricRepo domain.HealthMetricRepository,
	credRepo domain.VPNCredentialRepository,
	agentRepo domain.AgentRepository,
	agentCredRepo domain.AgentCredentialRepository,
	agentCmdRepo domain.AgentCommandRepository,
	sessionRepo domain.ExpressVPNSessionRepository,
	vpnMgr *vpn.VpnManager,
	proxyMgr *proxy.ProxyManager,
	heartbeatService *health.AgentHeartbeatService,
	metricsService *health.SystemMetricsService,
	validationService *vpn.ExpressVPNValidationService,
	healthMonitor *health.HealthMonitorService,
	customerService *customer.Service,
	billingService *billing.Service,
	abuseService *abuse.Service,
	routingService *routing.Service,
	dashboardWS *DashboardWSGateway,
	agentWS *AgentWSGateway,
	commandBus *event.CommandBus,
	eventBus *event.EventBus,
) *Handler {
	return &Handler{
		nodeRepo:          nodeRepo,
		proxyRepo:         proxyRepo,
		auditRepo:         auditRepo,
		metricRepo:        metricRepo,
		credRepo:          credRepo,
		agentRepo:         agentRepo,
		agentCredRepo:     agentCredRepo,
		agentCmdRepo:      agentCmdRepo,
		sessionRepo:       sessionRepo,
		vpnMgr:            vpnMgr,
		proxyMgr:          proxyMgr,
		locCache:          cache.NewLocationCache(24 * time.Hour),
		heartbeatService:  heartbeatService,
		metricsService:    metricsService,
		validationService: validationService,
		healthMonitor:     healthMonitor,
		customerService:   customerService,
		billingService:    billingService,
		abuseService:      abuseService,
		routingService:    routingService,
		dashboardWS:       dashboardWS,
		agentWS:           agentWS,
		commandBus:        commandBus,
		eventBus:          eventBus,
	}
}

// VPN handlers
func (h *Handler) ListVPNs(c *gin.Context) {
	nodes, err := h.nodeRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nodes)
}

type CreateVPNInput struct {
	Name       string `json:"name" binding:"required"`
	Provider   string `json:"provider"`
	Type       string `json:"type" binding:"required"` // wireguard, mock
	ConfigText string `json:"config_text"`
}

func (h *Handler) CreateVPN(c *gin.Context) {
	var input CreateVPNInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	node := &domain.VPNNode{
		ID:               uuid.New().String(),
		Name:             input.Name,
		Provider:         input.Provider,
		Type:             input.Type,
		ConfigText:       input.ConfigText,
		Status:           "disconnected",
		ConnectionStatus: "disconnected",
		AgentID:          "local-agent",
	}

	if err := h.nodeRepo.Create(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "VPN_CREATED",
		Details:   fmt.Sprintf("Created VPN node: %s (%s)", node.Name, node.Type),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusCreated, node)
}

func (h *Handler) ConnectVPN(c *gin.Context) {
	id := c.Param("id")
	inf, err := h.vpnMgr.Connect(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "Connected successfully",
		"interface": inf.GetInterfaceName(),
		"local_ip":  inf.GetLocalIP(),
	})
}

func (h *Handler) DisconnectVPN(c *gin.Context) {
	id := c.Param("id")
	if err := h.vpnMgr.Disconnect(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Disconnected successfully"})
}

func (h *Handler) DeleteVPN(c *gin.Context) {
	id := c.Param("id")
	// Make sure VPN is disconnected first
	h.vpnMgr.Disconnect(c.Request.Context(), id)

	// Fetch linked proxies and stop them
	proxies, err := h.proxyRepo.GetByVPNNodeID(id)
	if err == nil {
		for _, prxy := range proxies {
			h.proxyMgr.StopProxy(prxy.ID)
			h.proxyRepo.Delete(prxy.ID)
		}
	}

	if err := h.nodeRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "VPN node and associated proxies deleted successfully"})
}

// VPNCredential endpoints
type CreateCredentialInput struct {
	Provider string `json:"provider" binding:"required"`
	Name     string `json:"name" binding:"required"`
	Secret   string `json:"secret" binding:"required"`
}

func (h *Handler) CreateCredential(c *gin.Context) {
	var input CreateCredentialInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encrypted, err := security.EncryptSecret(input.Secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt secret key"})
		return
	}

	cred := &domain.VPNCredential{
		ID:              uuid.New().String(),
		Provider:        input.Provider,
		Name:            input.Name,
		EncryptedSecret: encrypted,
		MaskedSecret:    security.MaskSecret(input.Secret),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.credRepo.Create(cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, cred)
}

func (h *Handler) ListCredentials(c *gin.Context) {
	creds, err := h.credRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, creds)
}

// ExpressVPN Locations endpoint
type RegionGroup struct {
	Name      string            `json:"name"`
	Locations []vpn.VPNLocation `json:"locations"`
}

func (h *Handler) ListLocations(c *gin.Context) {
	driverType := c.DefaultQuery("type", "expressvpn") // expressvpn, expressvpn_mock

	if locs, found := h.locCache.Get(); found {
		c.JSON(http.StatusOK, gin.H{"regions": groupLocationsByRegion(locs)})
		return
	}

	locs, err := h.vpnMgr.ListLocations(c.Request.Context(), driverType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.locCache.Set(locs)
	c.JSON(http.StatusOK, gin.H{"regions": groupLocationsByRegion(locs)})
}

func groupLocationsByRegion(locs []vpn.VPNLocation) []RegionGroup {
	groupsMap := make(map[string][]vpn.VPNLocation)
	for _, l := range locs {
		groupsMap[l.Region] = append(groupsMap[l.Region], l)
	}

	var regions []RegionGroup
	ordered := []string{"Asia Pacific", "Americas", "Europe", "Middle East and Africa", "Other"}
	for _, name := range ordered {
		if items, exists := groupsMap[name]; exists {
			regions = append(regions, RegionGroup{Name: name, Locations: items})
			delete(groupsMap, name)
		}
	}

	for name, items := range groupsMap {
		regions = append(regions, RegionGroup{Name: name, Locations: items})
	}

	return regions
}

// ExpressVPN Node Creation endpoint
type CreateExpressVPNNodeInput struct {
	AgentID             string `json:"agentId"`
	Name                string `json:"name" binding:"required"`
	CredentialID        string `json:"credentialId" binding:"required"`
	LocationAlias       string `json:"locationAlias" binding:"required"`
	LocationDisplayName string `json:"locationDisplayName" binding:"required"`
	SelectedCountry     string `json:"selectedCountry"`
	SelectedRegion      string `json:"selectedRegion"`
	Protocol            string `json:"protocol"`
}

func (h *Handler) CreateExpressVPNNode(c *gin.Context) {
	var input CreateExpressVPNNodeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentID := input.AgentID
	if agentID == "" {
		agentID = "local-agent"
	}

	// Dynamic detection: if credential type or name implies mock, we use expressvpn_mock driver
	nodeType := "expressvpn"
	cred, err := h.credRepo.GetByID(input.CredentialID)
	if err == nil && (cred.Provider == "expressvpn_mock" || strings.Contains(strings.ToLower(cred.Name), "mock")) {
		nodeType = "expressvpn_mock"
	}

	node := &domain.VPNNode{
		ID:                  uuid.New().String(),
		Name:                input.Name,
		Provider:            "expressvpn",
		Type:                nodeType,
		Status:              "disconnected",
		AgentID:             agentID,
		CredentialID:        input.CredentialID,
		LocationAlias:       input.LocationAlias,
		LocationDisplayName: input.LocationDisplayName,
		SelectedCountry:     input.SelectedCountry,
		SelectedRegion:      input.SelectedRegion,
		Protocol:            input.Protocol,
		ConnectionStatus:    "disconnected",
	}

	if err := h.nodeRepo.Create(node); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "VPN_CREATED",
		Details:   fmt.Sprintf("Created ExpressVPN node: %s (%s)", node.Name, node.LocationDisplayName),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusCreated, node)
}

// Status endpoint
func (h *Handler) GetVPNStatus(c *gin.Context) {
	id := c.Param("id")
	node, err := h.nodeRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "VPN node not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"provider":          node.Provider,
		"agent":             node.AgentID,
		"selected_location": node.LocationDisplayName,
		"public_ip":         node.IP,
		"detected_country":  node.DetectedCountry,
		"connection_status": node.Status,
		"last_error":        node.LastError,
	})
}

// Connect & Create Proxy orchestrator
type ConnectAndCreateProxyInput struct {
	ProxyType    string `json:"proxyType" binding:"required"` // socks5, http
	Port         int    `json:"port" binding:"required"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	RotationMode string `json:"rotationMode"`
}

func (h *Handler) ConnectAndCreateProxy(c *gin.Context) {
	id := c.Param("id")

	// 1. Connect VPN
	_, err := h.vpnMgr.Connect(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to connect VPN: " + err.Error()})
		return
	}

	var input ConnectAndCreateProxyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 2. Fetch node & verify connection status
	node, err := h.nodeRepo.GetByID(id)
	if err != nil || node.Status != "connected" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "VPN connection is not marked connected"})
		return
	}

	// 3. Stop and replace any proxy currently occupying this port
	existing, _ := h.proxyRepo.GetByPort(input.Port)
	if existing != nil {
		h.proxyMgr.StopProxy(existing.ID)
		h.proxyRepo.Delete(existing.ID)
	}

	rotationMode := input.RotationMode
	if rotationMode == "" {
		rotationMode = "static"
	}

	// 4. Create and start proxy
	prxy := &domain.Proxy{
		ID:           uuid.New().String(),
		VPNNodeID:    node.ID,
		Port:         input.Port,
		Type:         input.ProxyType,
		BindIP:       "0.0.0.0",
		Username:     input.Username,
		Password:     input.Password,
		Status:       "stopped",
		AgentID:      node.AgentID,
		RotationMode: rotationMode,
	}

	if err := h.proxyRepo.Create(prxy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register proxy: " + err.Error()})
		return
	}

	if err := h.proxyMgr.StartProxy(prxy.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start proxy: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "VPN connected and proxy created successfully",
		"host":      "127.0.0.1",
		"port":      prxy.Port,
		"type":      prxy.Type,
		"username":  prxy.Username,
		"password":  prxy.Password,
		"status":    "running",
		"public_ip": node.IP,
	})
}

// Proxy handlers
func (h *Handler) ListProxies(c *gin.Context) {
	proxies, err := h.proxyRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, proxies)
}

type CreateProxyInput struct {
	VPNNodeID    string `json:"vpn_node_id"`
	Port         int    `json:"port" binding:"required"`
	Type         string `json:"type" binding:"required"` // socks5, http
	BindIP       string `json:"bind_ip"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	ExpiresHours int    `json:"expires_hours"`
	RotationMode string `json:"rotation_mode"`
}

func (h *Handler) CreateProxy(c *gin.Context) {
	var input CreateProxyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if input.BindIP == "" {
		input.BindIP = "0.0.0.0"
	}

	generatedPassword := ""
	if input.Username == "" || input.Password == "" {
		cred, err := proxy.NewProxyCredentialManager().GenerateCredential()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to generate proxy credential: %v", err)})
			return
		}
		input.Username = cred.Username
		input.Password = cred.Password
		generatedPassword = cred.Password
	}

	existing, _ := h.proxyRepo.GetByPort(input.Port)
	if existing != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Port %d is already in use", input.Port)})
		return
	}

	rotationMode := input.RotationMode
	if rotationMode == "" {
		rotationMode = "static"
	}

	expiresHours := input.ExpiresHours
	if expiresHours <= 0 {
		expiresHours = 12
	}
	expiresAt := time.Now().Add(time.Duration(expiresHours) * time.Hour)

	node, err := h.nodeRepo.GetByID(input.VPNNodeID)
	agentID := "local-agent"
	if err == nil && node != nil {
		agentID = node.AgentID
	}

	prxy := &domain.Proxy{
		ID:           uuid.New().String(),
		VPNNodeID:    input.VPNNodeID,
		Port:         input.Port,
		Type:         input.Type,
		BindIP:       input.BindIP,
		Username:     input.Username,
		Password:     input.Password,
		ExpiresAt:    &expiresAt,
		Status:       "stopped",
		AgentID:      agentID,
		RotationMode: rotationMode,
	}

	if err := h.proxyRepo.Create(prxy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"proxy":                prxy,
		"provisioned_password": generatedPassword,
		"expires_at":           prxy.ExpiresAt,
		"expires_in_hours":     expiresHours,
		"rental_mode":          true,
	})
}

func (h *Handler) StartProxy(c *gin.Context) {
	id := c.Param("id")
	if err := h.proxyMgr.StartProxy(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Proxy started successfully"})
}

func (h *Handler) StopProxy(c *gin.Context) {
	id := c.Param("id")
	if err := h.proxyMgr.StopProxy(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Proxy stopped successfully"})
}

func (h *Handler) DeleteProxy(c *gin.Context) {
	id := c.Param("id")
	h.proxyMgr.StopProxy(id)

	if err := h.proxyRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Proxy deleted successfully"})
}

func (h *Handler) GetSystemStats(c *gin.Context) {
	stats := h.metricsService.GetCurrent()
	grade, lat := h.healthMonitor.GetCurrentGrade()

	agents, _ := h.agentRepo.List()
	totalAgents := len(agents)
	onlineAgents := 0
	for _, a := range agents {
		if a.Status == "healthy" {
			onlineAgents++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"cpu_usage":          stats.CPUUsage,
		"ram_usage":          stats.RAMUsage,
		"disk_usage":         stats.DiskUsage,
		"net_in":             stats.NetIn,
		"net_out":            stats.NetOut,
		"active_vpn_count":   stats.VPNCount,
		"active_proxy_count": stats.ProxyCount,
		"health_grade":       grade,
		"health_latency":     lat,
		"total_agents":       totalAgents,
		"online_agents":      onlineAgents,
		"timestamp":          stats.Timestamp,
	})
}

func (h *Handler) GetLogs(c *gin.Context) {
	logs, err := h.auditRepo.List(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, logs)
}

func (h *Handler) GetProxyMetrics(c *gin.Context) {
	id := c.Param("id")
	metrics, err := h.metricRepo.GetLatest(id, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, metrics)
}

// Agent CRUD
func (h *Handler) RegisterAgent(c *gin.Context) {
	var input struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Hostname  string `json:"hostname"`
		IPAddress string `json:"ip_address"`
		OS        string `json:"os"`
		Version   string `json:"version"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentID := input.ID
	if agentID == "" {
		agentID = uuid.New().String()
	}

	agent := &domain.Agent{
		ID:              agentID,
		Name:            input.Name,
		Hostname:        input.Hostname,
		IPAddress:       input.IPAddress,
		OS:              input.OS,
		Version:         input.Version,
		Status:          "healthy",
		LastHeartbeatAt: time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.agentRepo.Create(agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "AGENT_REGISTERED",
		Details:   fmt.Sprintf("Agent registered: %s (ID: %s)", agent.Name, agent.ID),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusCreated, agent)
}

func (h *Handler) ListAgents(c *gin.Context) {
	agents, err := h.agentRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, agents)
}

func (h *Handler) GetAgent(c *gin.Context) {
	id := c.Param("id")
	agent, err := h.agentRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, agent)
}

func (h *Handler) DeleteAgent(c *gin.Context) {
	id := c.Param("id")
	if id == "local-agent" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete default local-agent"})
		return
	}
	if err := h.agentRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "AGENT_DELETED",
		Details:   fmt.Sprintf("Agent deleted: %s", id),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{"message": "Agent deleted successfully"})
}

func (h *Handler) ReceiveHeartbeat(c *gin.Context) {
	id := c.Param("id")
	var input struct {
		CPUUsage   float64 `json:"cpu_usage"`
		RAMUsage   float64 `json:"ram_usage"`
		VPNCount   int     `json:"vpn_count"`
		ProxyCount int     `json:"proxy_count"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	h.heartbeatService.RecordHeartbeat(id, input.CPUUsage, input.RAMUsage, input.VPNCount, input.ProxyCount)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) ValidateVPN(c *gin.Context) {
	id := c.Param("id")
	report, err := h.validationService.ValidateNode(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

// Prometheus stats helper
func (h *Handler) GetPrometheusMetrics(c *gin.Context) {
	stats := h.metricsService.GetCurrent()
	grade, lat := h.healthMonitor.GetCurrentGrade()

	c.Header("Content-Type", "text/plain; version=0.0.4")
	c.String(http.StatusOK,
		"# HELP vpn_proxy_cpu_usage Current CPU usage percentage\n"+
			"# TYPE vpn_proxy_cpu_usage gauge\n"+
			fmt.Sprintf("vpn_proxy_cpu_usage %f\n", stats.CPUUsage)+
			"# HELP vpn_proxy_ram_usage Current RAM usage percentage\n"+
			"# TYPE vpn_proxy_ram_usage gauge\n"+
			fmt.Sprintf("vpn_proxy_ram_usage %f\n", stats.RAMUsage)+
			"# HELP vpn_proxy_active_vpns Total active VPN connections\n"+
			"# TYPE vpn_proxy_active_vpns gauge\n"+
			fmt.Sprintf("vpn_proxy_active_vpns %d\n", stats.VPNCount)+
			"# HELP vpn_proxy_active_proxies Total active proxy servers\n"+
			"# TYPE vpn_proxy_active_proxies gauge\n"+
			fmt.Sprintf("vpn_proxy_active_proxies %d\n", stats.ProxyCount)+
			"# HELP vpn_proxy_latency_ms Global health latency in milliseconds\n"+
			"# TYPE vpn_proxy_latency_ms gauge\n"+
			fmt.Sprintf("vpn_proxy_latency_ms %d\n", lat)+
			"# HELP vpn_proxy_health_grade Global system health grade\n"+
			"# TYPE vpn_proxy_health_grade gauge\n"+
			fmt.Sprintf("vpn_proxy_health_grade{grade=\"%s\"} 1\n", grade),
	)
}

// ─── Phase 2B: Agent Registration with Token ────────────────────────────────

func generateToken() (plaintext string, hash string, err error) {
	b := make([]byte, 32) // 256 bits
	if _, err = rand.Read(b); err != nil {
		return
	}
	plaintext = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(h[:])
	return
}

// RegisterAgentWithToken: POST /api/agents/register
// Re-implements RegisterAgent to also issue a token. Returns plaintext once only.
func (h *Handler) RegisterAgentWithToken(c *gin.Context) {
	var input struct {
		ID        string `json:"id"`
		Name      string `json:"name" binding:"required"`
		Hostname  string `json:"hostname"`
		IPAddress string `json:"ip_address"`
		OS        string `json:"os"`
		Version   string `json:"version"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentID := input.ID
	if agentID == "" {
		agentID = uuid.New().String()
	}

	agent := &domain.Agent{
		ID:              agentID,
		Name:            input.Name,
		Hostname:        input.Hostname,
		IPAddress:       input.IPAddress,
		OS:              input.OS,
		Version:         input.Version,
		Status:          "offline",
		LastHeartbeatAt: time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := h.agentRepo.Create(agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	plain, hash, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	cred := &domain.AgentCredential{
		AgentID:   agentID,
		TokenHash: hash,
		IssuedAt:  time.Now(),
		ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
		CreatedAt: time.Now(),
	}
	if err := h.agentCredRepo.Create(cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential storage failed"})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "AGENT_REGISTERED",
		Details:   fmt.Sprintf("Agent %s (%s) registered with token", agent.Name, agentID),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusCreated, gin.H{
		"agentId": agentID,
		"agent":   agent,
		"token":   plain, // shown ONCE, never stored
	})
}

// RotateToken: POST /api/agents/:id/rotate-token
func (h *Handler) RotateToken(c *gin.Context) {
	agentID := c.Param("id")

	plain, hash, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	cred, err := h.agentCredRepo.GetByAgentID(agentID)
	if err != nil {
		// create new credential record if none existed
		cred = &domain.AgentCredential{
			AgentID:   agentID,
			IssuedAt:  time.Now(),
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
			CreatedAt: time.Now(),
		}
	}
	now := time.Now()
	cred.TokenHash = hash
	cred.IssuedAt = now
	cred.ExpiresAt = now.Add(365 * 24 * time.Hour)
	cred.RevokedAt = nil

	if err := h.agentCredRepo.Update(cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "credential update failed"})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "AGENT_TOKEN_ROTATED",
		Details:   fmt.Sprintf("Token rotated for agent %s", agentID),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusOK, gin.H{
		"agentId": agentID,
		"token":   plain, // shown ONCE
	})
}

// RevokeAgent: POST /api/agents/:id/revoke
func (h *Handler) RevokeAgent(c *gin.Context) {
	agentID := c.Param("id")

	cred, err := h.agentCredRepo.GetByAgentID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "credential not found"})
		return
	}
	now := time.Now()
	cred.RevokedAt = &now
	if err := h.agentCredRepo.Update(cred); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revocation failed"})
		return
	}

	// Mark agent offline
	if agent, err := h.agentRepo.GetByID(agentID); err == nil {
		agent.Status = "offline"
		agent.UpdatedAt = time.Now()
		_ = h.agentRepo.Update(agent)
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "AGENT_REVOKED",
		Details:   fmt.Sprintf("Agent %s token revoked", agentID),
		Timestamp: time.Now(),
	})

	// Notify CommandBus to drop WS connection
	h.commandBus.UnregisterAgentConnection(agentID)

	c.JSON(http.StatusOK, gin.H{"message": "agent revoked and disconnected"})
}

// ─── Phase 2B: Command Bus API ───────────────────────────────────────────────

var allowedCommandTypes = map[string]bool{
	"CONNECT_VPN":    true,
	"DISCONNECT_VPN": true,
	"CREATE_PROXY":   true,
	"DELETE_PROXY":   true,
	"RESTART_PROXY":  true,
	"HEALTH_CHECK":   true,
	"SYNC_STATE":     true,
}

// CreateAgentCommand: POST /api/agents/:id/commands
func (h *Handler) CreateAgentCommand(c *gin.Context) {
	agentID := c.Param("id")

	var input struct {
		Type           string      `json:"type" binding:"required"`
		Payload        interface{} `json:"payload"`
		TimeoutSeconds int         `json:"timeoutSeconds"`
		IdempotencyKey string      `json:"idempotencyKey"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !allowedCommandTypes[input.Type] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command type not allowed: " + input.Type})
		return
	}

	payloadBytes, _ := json.Marshal(input.Payload)

	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = 120
	}

	cmd := &domain.AgentCommand{
		ID:             uuid.New().String(),
		AgentID:        agentID,
		Type:           input.Type,
		Payload:        string(payloadBytes),
		IdempotencyKey: input.IdempotencyKey,
		TimeoutSeconds: timeout,
		MaxAttempts:    3,
	}

	if err := h.commandBus.Dispatch(cmd); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	h.auditRepo.Create(&domain.AuditLog{
		Action:    "COMMAND_CREATED",
		Details:   fmt.Sprintf("Command %s type=%s dispatched to agent %s", cmd.ID, cmd.Type, agentID),
		Timestamp: time.Now(),
	})

	c.JSON(http.StatusCreated, gin.H{"commandId": cmd.ID, "status": "pending"})
}

// GetCommandStatus: GET /api/commands/:id
func (h *Handler) GetCommandStatus(c *gin.Context) {
	id := c.Param("id")
	cmd, err := h.agentCmdRepo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
		return
	}
	c.JSON(http.StatusOK, cmd)
}

// ListAgentCommands: GET /api/agents/:id/commands
func (h *Handler) ListAgentCommands(c *gin.Context) {
	cmds, err := h.agentCmdRepo.ListAll(100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cmds)
}

// ListCommands: GET /api/commands
func (h *Handler) ListCommands(c *gin.Context) {
	cmds, err := h.agentCmdRepo.ListAll(500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cmds)
}

// ─── Phase 2B: Audit Export ──────────────────────────────────────────────────

// ExportAuditLog: GET /api/audit/export?format=json|csv
func (h *Handler) ExportAuditLog(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	logs, err := h.auditRepo.List(1000)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if format == "csv" {
		var buf bytes.Buffer
		w := csv.NewWriter(&buf)
		_ = w.Write([]string{"ID", "Action", "Details", "Timestamp"})
		for _, l := range logs {
			_ = w.Write([]string{
				fmt.Sprintf("%d", l.ID),
				l.Action,
				l.Details,
				l.Timestamp.Format(time.RFC3339),
			})
		}
		w.Flush()
		c.Header("Content-Disposition", "attachment; filename=audit_log.csv")
		c.Data(http.StatusOK, "text/csv", buf.Bytes())
		return
	}

	c.JSON(http.StatusOK, logs)
}

func (h *Handler) customerPrincipal(c *gin.Context) (*customer.Principal, bool) {
	principal, err := h.customerService.AuthenticateBearer(c.GetHeader("Authorization"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return nil, false
	}
	return principal, true
}

func (h *Handler) CustomerRegister(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	customer, token, err := h.customerService.Register(input.Email, input.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.billingService.Audit(customer.ID, "customer", "CUSTOMER_REGISTERED", customer.ID, "customer", map[string]interface{}{"email": customer.Email})
	c.JSON(http.StatusCreated, gin.H{"customer": customer, "token": token})
}

func (h *Handler) CustomerLogin(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	customer, token, err := h.customerService.Login(input.Email, input.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	h.billingService.Audit(customer.ID, "customer", "LOGIN", customer.ID, "customer", map[string]interface{}{"email": customer.Email})
	c.JSON(http.StatusOK, gin.H{"customer": customer, "token": token})
}

func (h *Handler) CustomerListPlans(c *gin.Context) {
	plans, err := h.customerService.ListPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plans)
}

func (h *Handler) CustomerListProxies(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	items, err := h.customerService.ListAllocatedProxies(principal.Customer.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) CustomerAllocateProxy(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	var input struct {
		Country      string `json:"country"`
		RotationMode string `json:"rotationMode"`
		Type         string `json:"type"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	item, err := h.customerService.AllocateProxy(principal.Customer.ID, input.Country, input.RotationMode, input.Type)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.billingService.Audit(principal.Customer.ID, "customer", "PROXY_ALLOCATION", item.ID, "proxy_allocation", map[string]interface{}{"country": input.Country, "type": input.Type, "rotation_mode": input.RotationMode})
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) CustomerRotateCredential(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	item, err := h.customerService.RotateCredential(principal.Customer.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) CustomerRotateProxy(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	item, err := h.customerService.RotateProxy(principal.Customer.ID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) CustomerReleaseProxy(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	if err := h.customerService.ReleaseProxy(principal.Customer.ID, c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.billingService.Audit(principal.Customer.ID, "customer", "PROXY_DELETION", c.Param("id"), "proxy_allocation", nil)
	c.JSON(http.StatusOK, gin.H{"message": "proxy allocation released"})
}

func (h *Handler) CustomerUsage(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	usage, err := h.customerService.Usage(principal.Customer.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (h *Handler) CustomerCreateAPIKey(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&input)
	key, plain, err := h.customerService.CreateAPIKey(principal.Customer.ID, input.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"apiKey": key, "key": plain})
}

func (h *Handler) CustomerDeleteAPIKey(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	if err := h.customerService.DeleteAPIKey(principal.Customer.ID, c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "api key revoked"})
}

func (h *Handler) CustomerExportProxies(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	format := c.DefaultQuery("format", "txt")
	contentType, body, err := h.customerService.ExportProxies(principal.Customer.ID, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, contentType, []byte(body))
}

func (h *Handler) AdminCreatePlan(c *gin.Context) {
	var plan domain.ProxyPlan
	if err := c.ShouldBindJSON(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.customerService.CreatePlan(&plan); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, plan)
}

func (h *Handler) AdminCreateSubscription(c *gin.Context) {
	var input struct {
		PlanID string `json:"planId" binding:"required"`
		Days   int    `json:"days"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.customerService.ActivateSubscription(c.Param("id"), input.PlanID, input.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, sub)
}

func (h *Handler) CustomerSubscriptionUsage(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	usage, err := h.billingService.UsageDashboard(principal.Customer.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (h *Handler) AdminBillingOverview(c *gin.Context) {
	overview, err := h.billingService.BillingOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	subs, _ := h.billingService.ListSubscriptions(100)
	invoices, _ := h.billingService.ListInvoices(100)
	overview["subscriptions"] = subs
	overview["invoices"] = invoices
	c.JSON(http.StatusOK, overview)
}

func (h *Handler) AdminProductionMetrics(c *gin.Context) {
	overview, err := h.billingService.BillingOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	stats := h.metricsService.GetCurrent()
	grade, _ := h.healthMonitor.GetCurrentGrade()
	agents, _ := h.agentRepo.List()
	onlineAgents := 0
	for _, agent := range agents {
		if agent.Status == "healthy" {
			onlineAgents++
		}
	}
	overview["running_proxies"] = stats.ProxyCount
	overview["online_agents"] = onlineAgents
	overview["health_score"] = grade
	c.JSON(http.StatusOK, overview)
}

func (h *Handler) AdminListCommercialPlans(c *gin.Context) {
	plans, err := h.billingService.ListPlans()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plans)
}

func (h *Handler) AdminCreateCommercialPlan(c *gin.Context) {
	var input struct {
		Plan     domain.Plan       `json:"plan"`
		Features map[string]string `json:"features"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if input.Plan.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "plan.name is required"})
		return
	}
	if err := h.billingService.CreatePlan(&input.Plan, input.Features); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, input.Plan)
}

func (h *Handler) AdminUpdateCommercialPlan(c *gin.Context) {
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plan, err := h.billingService.UpdatePlan(c.Param("id"), updates)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, plan)
}

func (h *Handler) AdminCreateCommercialSubscription(c *gin.Context) {
	var input struct {
		CustomerID string `json:"customerId" binding:"required"`
		PlanID     string `json:"planId" binding:"required"`
		Days       int    `json:"days"`
		AutoRenew  bool   `json:"autoRenew"`
		Status     string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.billingService.CreateSubscription(input.CustomerID, input.PlanID, input.Days, input.AutoRenew, input.Status)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, sub)
}

func (h *Handler) AdminUpdateSubscriptionStatus(c *gin.Context) {
	var input struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.billingService.UpdateSubscriptionStatus(c.Param("id"), input.Status); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "subscription status updated"})
}

func (h *Handler) AdminGenerateInvoice(c *gin.Context) {
	var input struct {
		CustomerID     string `json:"customerId" binding:"required"`
		SubscriptionID string `json:"subscriptionId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	invoice, err := h.billingService.GenerateInvoice(input.CustomerID, input.SubscriptionID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, invoice)
}

func (h *Handler) AdminMarkInvoicePaid(c *gin.Context) {
	if err := h.billingService.MarkInvoicePaid(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.billingService.Audit("admin", "system", "INVOICE_MARKED_PAID", c.Param("id"), "invoice", nil)
	c.JSON(http.StatusOK, gin.H{"message": "invoice marked paid"})
}

func (h *Handler) AdminVerifyInvoicePayment(c *gin.Context) {
	invoice, err := h.billingService.VerifyInvoicePayment(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, invoice)
}

func (h *Handler) AdminRefundInvoice(c *gin.Context) {
	invoice, err := h.billingService.RefundInvoice(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, invoice)
}

func (h *Handler) AdminInvoicePaymentStatus(c *gin.Context) {
	status, err := h.billingService.InvoicePaymentStatus(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) AdminSuspendCustomer(c *gin.Context) {
	if err := h.billingService.SuspendCustomer(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer suspended"})
}

func (h *Handler) AdminActivateCustomer(c *gin.Context) {
	if err := h.billingService.ActivateCustomer(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer activated"})
}

func (h *Handler) AdminListAuditEvents(c *gin.Context) {
	events, err := h.billingService.ListAuditEvents(500)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

func (h *Handler) AdminExportBackup(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	contentType, payload, err := h.billingService.ExportBackup(format, []string{"config.production.yaml", "docker-compose.yml"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	filename := "backup.json"
	if format == "zip" {
		filename = "backup.zip"
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(http.StatusOK, contentType, payload)
}

func (h *Handler) RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		method := c.Request.Method
		identity := c.ClientIP()
		limit := 120
		scope := "api"

		if strings.Contains(path, "/customer/login") || strings.Contains(path, "/customer/register") {
			limit = 10
			scope = "login"
		} else if strings.Contains(path, "/agents/") || strings.HasSuffix(path, "/agents") || strings.Contains(path, "/commands") || strings.Contains(path, "/ws/agent") {
			limit = 2000
			scope = "agent-control"
		} else if strings.Contains(path, "/system/") || strings.Contains(path, "/prometheus/") {
			limit = 600
			scope = "system-observability"
		} else if strings.Contains(path, "/customer/proxies/allocate") {
			limit = 20
			scope = "proxy-allocation"
			if p, err := h.customerService.AuthenticateBearer(c.GetHeader("Authorization")); err == nil {
				identity = p.Customer.ID
			}
		} else if strings.Contains(path, "/admin/") {
			limit = 300
			scope = "admin"
		} else if strings.Contains(path, "/customer/") {
			scope = "customer-api"
			if p, err := h.customerService.AuthenticateBearer(c.GetHeader("Authorization")); err == nil {
				identity = p.Customer.ID
				if p.APIKeyID != "" {
					identity = p.APIKeyID
				}
			}
		}

		if method != "OPTIONS" && !h.abuseService.CheckAPIRate(identity, scope, limit, time.Minute) {
			h.billingService.Audit(identity, "api", "RATE_LIMIT_TRIGGERED", path, "api", map[string]interface{}{"scope": scope, "limit": limit})
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *Handler) CustomerListIPWhitelist(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	rows, err := h.abuseService.ListWhitelist(principal.Customer.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) CustomerAddIPWhitelist(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	var input struct {
		IPAddress   string `json:"ipAddress"`
		CIDR        string `json:"cidr"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := h.abuseService.AddWhitelist(principal.Customer.ID, input.IPAddress, input.CIDR, input.Description)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) CustomerDeleteIPWhitelist(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	if err := h.abuseService.DeleteWhitelist(principal.Customer.ID, c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "whitelist entry deleted"})
}

func (h *Handler) CustomerSecurityDashboard(c *gin.Context) {
	principal, ok := h.customerPrincipal(c)
	if !ok {
		return
	}
	whitelist, _ := h.abuseService.ListWhitelist(principal.Customer.ID)
	events, _ := h.abuseService.RecentEvents(100)
	customerEvents := make([]domain.AbuseEvent, 0)
	for _, e := range events {
		if e.CustomerID == principal.Customer.ID {
			customerEvents = append(customerEvents, e)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"ip_whitelist":        whitelist,
		"recent_abuse_events": customerEvents,
	})
}

func (h *Handler) AdminListBlockedTargets(c *gin.Context) {
	rows, err := h.abuseService.ListBlockedTargets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) AdminAddBlockedTarget(c *gin.Context) {
	var input struct {
		Type   string `json:"type" binding:"required"`
		Value  string `json:"value" binding:"required"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	row, err := h.abuseService.AddBlockedTarget(input.Type, input.Value, input.Reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, row)
}

func (h *Handler) AdminDeleteBlockedTarget(c *gin.Context) {
	if err := h.abuseService.DeleteBlockedTarget(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "blocked target deleted"})
}

func (h *Handler) AdminAbuseDashboard(c *gin.Context) {
	data, err := h.abuseService.Dashboard()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handler) AdminAbuseSuspendCustomer(c *gin.Context) {
	if err := h.abuseService.SuspendCustomer(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer suspended"})
}

func (h *Handler) AdminAbuseUnsuspendCustomer(c *gin.Context) {
	if err := h.abuseService.UnsuspendCustomer(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "customer unsuspended"})
}

func (h *Handler) AdminAbuseSuspendProxy(c *gin.Context) {
	if err := h.abuseService.SuspendProxy(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "proxy suspended"})
}

func (h *Handler) AdminAbuseDisableCredential(c *gin.Context) {
	if err := h.abuseService.DisableCredential(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "credential disabled"})
}

func (h *Handler) AdminAbuseClearRisk(c *gin.Context) {
	if err := h.abuseService.ClearRisk(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "risk cleared"})
}

func (h *Handler) ListPools(c *gin.Context) {
	pools, err := h.routingService.ListPools()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pools)
}

func (h *Handler) GetPool(c *gin.Context) {
	pool, err := h.routingService.PoolOverview(c.Param("country"), true)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, pool)
}

func (h *Handler) GetPoolHealth(c *gin.Context) {
	pool, err := h.routingService.PoolOverview(c.Param("country"), false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"country":            pool.Pool.Country,
		"health":             pool.Health,
		"available_proxies":  pool.AvailableProxies,
		"pool_size":          pool.PoolSize,
		"average_quality":    pool.AverageQuality,
		"agent_redundancy":   pool.AgentRedundancy,
		"country_redundancy": pool.CountryRedundancy,
		"active_sessions":    pool.ActiveSessions,
	})
}

func (h *Handler) AdminRoutingDashboard(c *gin.Context) {
	data, err := h.routingService.Dashboard()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *Handler) AdminCreatePool(c *gin.Context) {
	var input struct {
		Country  string `json:"country" binding:"required"`
		Region   string `json:"region"`
		Strategy string `json:"strategy"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.routingService.CreatePool(input.Country, input.Region, input.Strategy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "pool created"})
}

func (h *Handler) AdminSyncPools(c *gin.Context) {
	if err := h.routingService.SyncPools(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "pools synced"})
}

func (h *Handler) AdminSelectProxy(c *gin.Context) {
	var input domain.RoutingSelectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selected, err := h.routingService.SelectProxy(input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, selected)
}

func (h *Handler) AdminRotateAllocation(c *gin.Context) {
	var input struct {
		CustomerID string `json:"customerId" binding:"required"`
		Reason     string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selected, err := h.routingService.RotateAllocation(input.CustomerID, c.Param("id"), input.Reason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, selected)
}

func (h *Handler) AdminFailoverAllocation(c *gin.Context) {
	var input struct {
		CustomerID string `json:"customerId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	selected, err := h.routingService.FailoverAllocation(input.CustomerID, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, selected)
}

func (h *Handler) AdminCreateReservation(c *gin.Context) {
	var input domain.ProxyReservation
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.routingService.CreateReservation(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, input)
}
