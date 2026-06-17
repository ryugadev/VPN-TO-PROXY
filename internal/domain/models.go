package domain

import (
	"time"
)

type VPNCredential struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Provider        string    `gorm:"not null" json:"provider"`
	Name            string    `gorm:"not null" json:"name"`
	EncryptedSecret string    `gorm:"not null" json:"-"`
	MaskedSecret    string    `gorm:"not null" json:"masked_secret"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type VPNNode struct {
	ID              string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name            string     `gorm:"not null" json:"name"`
	Provider        string     `json:"provider"`
	Type            string     `gorm:"not null" json:"type"` // wireguard, openvpn, mock, expressvpn, expressvpn_mock
	ConfigText      string     `gorm:"type:text" json:"config_text"`
	Status          string     `gorm:"not null;default:'disconnected'" json:"status"` // connected, disconnected, connecting, reconnecting, failed
	InterfaceName   string     `json:"interface_name"`
	LocalIP         string     `json:"local_ip"`
	IP              string     `json:"ip"`
	Country         string     `json:"country"`
	ISP             string     `json:"isp"`
	ASN             string     `json:"asn"`
	Region          string     `json:"region"`
	LatencyMs       int64      `json:"latency_ms"`
	LastConnectedAt *time.Time `json:"last_connected_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// ExpressVPN specific fields
	AgentID             string `json:"agent_id"`
	CredentialID        string `json:"credential_id"`
	LocationAlias       string `json:"location_alias"`
	LocationDisplayName string `json:"location_display_name"`
	SelectedCountry     string `json:"selected_country"`
	SelectedRegion      string `json:"selected_region"`
	Protocol            string `json:"protocol"`
	PublicIP            string `json:"public_ip"`
	DetectedCountry     string `json:"detected_country"`
	AssignedInterface   string `json:"assigned_interface"`
	ConnectionStatus    string `json:"connection_status"`
	LastError           string `json:"last_error"`
}

type Agent struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name            string    `gorm:"not null" json:"name"`
	Hostname        string    `json:"hostname"`
	IPAddress       string    `json:"ip_address"`
	OS              string    `json:"os"`
	Version         string    `json:"version"`
	Status          string    `json:"status"` // healthy, warning, offline
	LastHeartbeatAt time.Time `json:"last_heartbeat_at"`
	VPNCount        int       `json:"vpn_count"`
	ProxyCount      int       `json:"proxy_count"`
	CPUUsage        float64   `json:"cpu_usage"`
	RAMUsage        float64   `json:"ram_usage"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ExpressVPNSession struct {
	ID                  string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	AgentID             string     `gorm:"index" json:"agent_id"`
	NodeID              string     `gorm:"index" json:"node_id"`
	LocationAlias       string     `json:"location_alias"`
	LocationDisplayName string     `json:"location_display_name"`
	PublicIP            string     `json:"public_ip"`
	DetectedCountry     string     `json:"detected_country"`
	Status              string     `json:"status"`
	ConnectedAt         time.Time  `json:"connected_at"`
	DisconnectedAt      *time.Time `json:"disconnected_at"`
	LastError           string     `json:"last_error"`
}

type Proxy struct {
	ID        string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	VPNNodeID string     `gorm:"index" json:"vpn_node_id"`
	Port      int        `gorm:"uniqueIndex" json:"port"`
	Type      string     `gorm:"not null" json:"type"`                     // socks5, http
	Status    string     `gorm:"not null;default:'stopped'" json:"status"` // running, stopped, error
	BindIP    string     `gorm:"default:'0.0.0.0'" json:"bind_ip"`
	Username  string     `json:"username"`
	Password  string     `json:"password"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	Host      string     `json:"host"`
	PublicIP  string     `json:"public_ip"`
	Country   string     `json:"country"`
	Region    string     `json:"region"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// Extended fields
	AgentID      string `json:"agent_id"`
	RotationMode string `json:"rotation_mode"` // static, sticky_30m, etc.
}

type HealthMetric struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TargetID   string    `gorm:"index" json:"target_id"` // VPN Node ID or Proxy ID
	TargetType string    `json:"target_type"`            // vpn, proxy
	LatencyMs  int64     `json:"latency_ms"`
	Status     string    `json:"status"` // online, offline
	CheckedAt  time.Time `json:"checked_at"`
}

type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Action    string    `json:"action"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

type SystemConfig struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `json:"value"`
}

type AgentHeartbeat struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	AgentID    string    `gorm:"index" json:"agent_id"`
	CPUUsage   float64   `json:"cpu_usage"`
	RAMUsage   float64   `json:"ram_usage"`
	VPNCount   int       `json:"vpn_count"`
	ProxyCount int       `json:"proxy_count"`
	Timestamp  time.Time `json:"timestamp"`
}

type PortAllocation struct {
	Port      int       `gorm:"primaryKey" json:"port"`
	Purpose   string    `json:"purpose"`   // e.g. "proxy"
	TargetID  string    `json:"target_id"` // Proxy ID
	Status    string    `json:"status"`    // allocated, reserved
	CreatedAt time.Time `json:"created_at"`
}

type RotationPolicy struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name        string    `gorm:"not null" json:"name"`
	Mode        string    `gorm:"not null" json:"mode"` // static, sticky_30m, sticky_6h, sticky_24h, rotating
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ExpressVPNValidation struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	NodeID    string    `gorm:"index" json:"node_id"`
	Status    string    `json:"status"` // success, failed
	Details   string    `gorm:"type:text" json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

type SystemMetricSnapshot struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CPUUsage   float64   `json:"cpu_usage"`
	RAMUsage   float64   `json:"ram_usage"`
	DiskUsage  float64   `json:"disk_usage"`
	NetIn      uint64    `json:"net_in"`
	NetOut     uint64    `json:"net_out"`
	VPNCount   int       `json:"vpn_count"`
	ProxyCount int       `json:"proxy_count"`
	Timestamp  time.Time `json:"timestamp"`
}

type AgentCredential struct {
	AgentID   string     `gorm:"primaryKey;type:varchar(36)" json:"agent_id"`
	TokenHash string     `gorm:"not null;index" json:"-"`
	IssuedAt  time.Time  `json:"issued_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type AgentCommand struct {
	ID             string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	AgentID        string     `gorm:"index;type:varchar(36)" json:"agent_id"`
	Type           string     `gorm:"not null" json:"type"`
	Payload        string     `gorm:"type:text" json:"payload"`
	Status         string     `gorm:"not null;default:'pending'" json:"status"`
	IdempotencyKey string     `gorm:"index" json:"idempotency_key"`
	TimeoutSeconds int        `gorm:"default:120" json:"timeout_seconds"`
	Attempts       int        `gorm:"default:0" json:"attempts"`
	MaxAttempts    int        `gorm:"default:3" json:"max_attempts"`
	CreatedAt      time.Time  `json:"created_at"`
	DispatchedAt   *time.Time `json:"dispatched_at,omitempty"`
	ExecutedAt     *time.Time `json:"executed_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Result         string     `gorm:"type:text" json:"result,omitempty"`
	LastError      string     `gorm:"type:text" json:"last_error,omitempty"`
}

type Customer struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Email        string    `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Status       string    `gorm:"not null;default:'active'" json:"status"`
	Role         string    `gorm:"not null;default:'customer'" json:"role"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CustomerCredential struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID   string    `gorm:"index;not null" json:"customer_id"`
	PasswordHash string    `gorm:"not null" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CustomerSession struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string     `gorm:"index;not null" json:"customer_id"`
	TokenHash  string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type CustomerApiKey struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string     `gorm:"index;not null" json:"customer_id"`
	Name       string     `json:"name"`
	KeyHash    string     `gorm:"uniqueIndex;not null" json:"-"`
	Prefix     string     `json:"prefix"`
	Status     string     `gorm:"not null;default:'active'" json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type ProxyPlan struct {
	ID                    string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name                  string    `gorm:"uniqueIndex;not null" json:"name"`
	MaxProxies            int       `json:"max_proxies"`
	AllowedCountries      string    `gorm:"type:text" json:"allowed_countries"`
	BandwidthLimitGB      int       `json:"bandwidth_limit_gb"`
	RotationModes         string    `gorm:"type:text" json:"rotation_modes"`
	ConcurrentConnections int       `json:"concurrent_connections"`
	Price                 float64   `json:"price"`
	Status                string    `gorm:"not null;default:'active'" json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type CustomerSubscription struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID   string    `gorm:"index;not null" json:"customer_id"`
	PlanID       string    `gorm:"index;not null" json:"plan_id"`
	Status       string    `gorm:"not null;default:'active'" json:"status"`
	StartsAt     time.Time `json:"starts_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	UsageResetAt time.Time `json:"usage_reset_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type CustomerProxyCredential struct {
	ID           string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Username     string     `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string     `gorm:"not null" json:"-"`
	CustomerID   string     `gorm:"index;not null" json:"customer_id"`
	ProxyID      string     `gorm:"index;not null" json:"proxy_id"`
	Status       string     `gorm:"not null;default:'active'" json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	RotatedAt    *time.Time `json:"rotated_at,omitempty"`
}

type CustomerProxyAllocation struct {
	ID             string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID     string     `gorm:"index;not null" json:"customer_id"`
	SubscriptionID string     `gorm:"index;not null" json:"subscription_id"`
	ProxyID        string     `gorm:"index;not null" json:"proxy_id"`
	CredentialID   string     `gorm:"index;not null" json:"credential_id"`
	RotationMode   string     `gorm:"not null;default:'static'" json:"rotation_mode"`
	Country        string     `json:"country"`
	Status         string     `gorm:"not null;default:'active'" json:"status"`
	BandwidthIn    uint64     `json:"bandwidth_in"`
	BandwidthOut   uint64     `json:"bandwidth_out"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DegradedAt     *time.Time `json:"degraded_at,omitempty"`
}

type UsageMetric struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	CustomerID        string    `gorm:"index" json:"customer_id"`
	ProxyID           string    `gorm:"index" json:"proxy_id"`
	AgentID           string    `gorm:"index" json:"agent_id"`
	BandwidthIn       uint64    `json:"bandwidth_in"`
	BandwidthOut      uint64    `json:"bandwidth_out"`
	ConnectionCount   int       `json:"connection_count"`
	RequestCount      int       `json:"request_count"`
	ActiveConnections int       `json:"active_connections"`
	Bucket            string    `gorm:"index" json:"bucket"`
	PeriodStart       time.Time `json:"period_start"`
	CreatedAt         time.Time `json:"created_at"`
}

type Invoice struct {
	ID             string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID     string     `gorm:"index" json:"customer_id"`
	PlanID         string     `gorm:"index" json:"plan_id"`
	SubscriptionID string     `gorm:"index" json:"subscription_id"`
	Amount         float64    `json:"amount"`
	Currency       string     `json:"currency"`
	Status         string     `json:"status"`
	Provider       string     `json:"provider"`
	PaymentRef     string     `json:"payment_ref"`
	CheckoutURL    string     `json:"checkout_url"`
	CreatedAt      time.Time  `json:"created_at"`
	PaidAt         *time.Time `json:"paid_at,omitempty"`
}

type PaymentProvider struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type BillingEvent struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string     `gorm:"index" json:"customer_id"`
	PlanID     string     `gorm:"index" json:"plan_id"`
	Amount     float64    `json:"amount"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	PaidAt     *time.Time `json:"paid_at,omitempty"`
}

type Plan struct {
	ID                    string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name                  string    `gorm:"uniqueIndex;not null" json:"name"`
	Description           string    `gorm:"type:text" json:"description"`
	Price                 float64   `json:"price"`
	Currency              string    `gorm:"not null;default:'USD'" json:"currency"`
	MaxProxies            int       `json:"max_proxies"`
	BandwidthLimitGB      int       `json:"bandwidth_limit_gb"`
	ConcurrentConnections int       `json:"concurrent_connections"`
	AllowedCountries      string    `gorm:"type:text" json:"allowed_countries"`
	Status                string    `gorm:"not null;default:'active'" json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type PlanFeature struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	PlanID    string    `gorm:"index;not null" json:"plan_id"`
	Key       string    `gorm:"not null" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	CreatedAt time.Time `json:"created_at"`
}

type Subscription struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string    `gorm:"index;not null" json:"customer_id"`
	PlanID     string    `gorm:"index;not null" json:"plan_id"`
	Status     string    `gorm:"not null;default:'pending'" json:"status"`
	StartsAt   time.Time `json:"starts_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	AutoRenew  bool      `json:"auto_renew"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type InvoiceItem struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	InvoiceID   string    `gorm:"index;not null" json:"invoice_id"`
	Description string    `gorm:"type:text" json:"description"`
	Quantity    int       `json:"quantity"`
	UnitAmount  float64   `json:"unit_amount"`
	Amount      float64   `json:"amount"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditEvent struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ActorID    string    `gorm:"index" json:"actor_id"`
	ActorType  string    `json:"actor_type"`
	Action     string    `gorm:"index;not null" json:"action"`
	TargetID   string    `gorm:"index" json:"target_id"`
	TargetType string    `json:"target_type"`
	Metadata   string    `gorm:"type:text" json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

type AbuseRule struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	Type      string    `gorm:"index;not null" json:"type"`
	Threshold int       `json:"threshold"`
	Action    string    `json:"action"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AbuseEvent struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string    `gorm:"index" json:"customer_id"`
	ProxyID    string    `gorm:"index" json:"proxy_id"`
	RuleID     string    `gorm:"index" json:"rule_id"`
	Severity   string    `gorm:"index" json:"severity"`
	Message    string    `gorm:"type:text" json:"message"`
	Metadata   string    `gorm:"type:text" json:"metadata"`
	CreatedAt  time.Time `json:"created_at"`
}

type CustomerRiskScore struct {
	CustomerID string    `gorm:"primaryKey;type:varchar(36)" json:"customer_id"`
	Score      int       `json:"score"`
	Level      string    `gorm:"index" json:"level"`
	Factors    string    `gorm:"type:text" json:"factors"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type IPWhitelist struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID  string    `gorm:"index;not null" json:"customer_id"`
	IPAddress   string    `json:"ip_address"`
	CIDR        string    `json:"cidr"`
	Description string    `json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type BlockedTarget struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Type      string    `gorm:"index;not null" json:"type"`
	Value     string    `gorm:"index;not null" json:"value"`
	Reason    string    `json:"reason"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConnectionLimit struct {
	ID                   string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID           string    `gorm:"index" json:"customer_id"`
	ProxyID              string    `gorm:"index" json:"proxy_id"`
	MaxConcurrent        int       `json:"max_concurrent"`
	MaxPerProxy          int       `json:"max_per_proxy"`
	MaxFailedAuth        int       `json:"max_failed_auth"`
	MaxRequestsPerMinute int       `json:"max_requests_per_minute"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type ProxyPool struct {
	ID        string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Country   string    `gorm:"uniqueIndex;not null" json:"country"`
	Region    string    `gorm:"index" json:"region"`
	Status    string    `gorm:"index;not null;default:'active'" json:"status"`
	Strategy  string    `gorm:"not null;default:'weighted_score'" json:"strategy"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProxyPoolMember struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	PoolID       string    `gorm:"index;not null" json:"pool_id"`
	ProxyID      string    `gorm:"uniqueIndex;not null" json:"proxy_id"`
	AgentID      string    `gorm:"index" json:"agent_id"`
	VPNNodeID    string    `gorm:"index" json:"vpn_node_id"`
	HealthStatus string    `gorm:"index" json:"health_status"`
	QualityScore int       `json:"quality_score"`
	ActiveLoad   int       `json:"active_load"`
	Status       string    `gorm:"index;not null;default:'active'" json:"status"`
	LastChecked  time.Time `json:"last_checked"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ProxyQualitySnapshot struct {
	ID                    string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	ProxyID               string    `gorm:"index;not null" json:"proxy_id"`
	Score                 int       `gorm:"index" json:"score"`
	Grade                 string    `gorm:"index" json:"grade"`
	LatencyMs             int64     `json:"latency_ms"`
	UptimePercent         float64   `json:"uptime_percent"`
	ConnectionSuccessRate float64   `json:"connection_success_rate"`
	BandwidthAvailableGB  float64   `json:"bandwidth_available_gb"`
	RecentFailures        int       `json:"recent_failures"`
	HealthStatus          string    `json:"health_status"`
	CreatedAt             time.Time `json:"created_at"`
}

type StickySession struct {
	ID           string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID   string    `gorm:"index;not null" json:"customer_id"`
	SessionKey   string    `gorm:"index;not null" json:"session_key"`
	Country      string    `gorm:"index" json:"country"`
	ProxyID      string    `gorm:"index;not null" json:"proxy_id"`
	RotationMode string    `json:"rotation_mode"`
	ExpiresAt    time.Time `gorm:"index" json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RoutingPolicy struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string    `gorm:"index" json:"customer_id"`
	Name       string    `gorm:"not null" json:"name"`
	Country    string    `gorm:"index" json:"country"`
	Region     string    `gorm:"index" json:"region"`
	Provider   string    `gorm:"index" json:"provider"`
	ASN        string    `gorm:"index" json:"asn"`
	ISP        string    `gorm:"index" json:"isp"`
	Strategy   string    `json:"strategy"`
	Enabled    bool      `gorm:"default:true" json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProxyReservation struct {
	ID         string     `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string     `gorm:"index;not null" json:"customer_id"`
	ProxyID    string     `gorm:"index" json:"proxy_id"`
	PoolID     string     `gorm:"index" json:"pool_id"`
	AgentID    string     `gorm:"index" json:"agent_id"`
	Country    string     `gorm:"index" json:"country"`
	Type       string     `gorm:"index;not null" json:"type"`
	Status     string     `gorm:"index;not null;default:'active'" json:"status"`
	StartsAt   time.Time  `json:"starts_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	Permanent  bool       `json:"permanent"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type RoutingEvent struct {
	ID         string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CustomerID string    `gorm:"index" json:"customer_id"`
	ProxyID    string    `gorm:"index" json:"proxy_id"`
	PoolID     string    `gorm:"index" json:"pool_id"`
	Action     string    `gorm:"index;not null" json:"action"`
	Message    string    `gorm:"type:text" json:"message"`
	Metadata   string    `gorm:"type:text" json:"metadata"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

type RoutingSelectionInput struct {
	CustomerID   string `json:"customer_id"`
	Country      string `json:"country"`
	Region       string `json:"region"`
	Provider     string `json:"provider"`
	ASN          string `json:"asn"`
	ISP          string `json:"isp"`
	Strategy     string `json:"strategy"`
	ProxyType    string `json:"proxy_type"`
	PlanID       string `json:"plan_id"`
	SessionKey   string `json:"session_key"`
	RotationMode string `json:"rotation_mode"`
	TargetRegion string `json:"target_region"`
}
