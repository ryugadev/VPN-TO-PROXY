package domain

type VPNNodeRepository interface {
	Create(node *VPNNode) error
	Update(node *VPNNode) error
	Delete(id string) error
	GetByID(id string) (*VPNNode, error)
	List() ([]VPNNode, error)
}

type ProxyRepository interface {
	Create(proxy *Proxy) error
	Update(proxy *Proxy) error
	Delete(id string) error
	GetByID(id string) (*Proxy, error)
	GetByPort(port int) (*Proxy, error)
	List() ([]Proxy, error)
	GetByVPNNodeID(vpnNodeID string) ([]Proxy, error)
}

type AuditLogRepository interface {
	Create(log *AuditLog) error
	List(limit int) ([]AuditLog, error)
}

type HealthMetricRepository interface {
	Create(metric *HealthMetric) error
	GetLatest(targetID string, limit int) ([]HealthMetric, error)
}

type VPNCredentialRepository interface {
	Create(cred *VPNCredential) error
	Update(cred *VPNCredential) error
	Delete(id string) error
	GetByID(id string) (*VPNCredential, error)
	List() ([]VPNCredential, error)
}

type AgentRepository interface {
	Create(agent *Agent) error
	Update(agent *Agent) error
	Delete(id string) error
	GetByID(id string) (*Agent, error)
	List() ([]Agent, error)
}

type ExpressVPNSessionRepository interface {
	Create(session *ExpressVPNSession) error
	Update(session *ExpressVPNSession) error
	GetByID(id string) (*ExpressVPNSession, error)
	GetActiveByAgent(agentID string) (*ExpressVPNSession, error)
	List() ([]ExpressVPNSession, error)
}

type AgentHeartbeatRepository interface {
	Create(hb *AgentHeartbeat) error
	ListByAgent(agentID string, limit int) ([]AgentHeartbeat, error)
}

type PortAllocationRepository interface {
	Create(alloc *PortAllocation) error
	GetByPort(port int) (*PortAllocation, error)
	Delete(port int) error
	List() ([]PortAllocation, error)
}

type RotationPolicyRepository interface {
	Create(policy *RotationPolicy) error
	Update(policy *RotationPolicy) error
	Delete(id string) error
	GetByID(id string) (*RotationPolicy, error)
	List() ([]RotationPolicy, error)
}

type ExpressVPNValidationRepository interface {
	Create(report *ExpressVPNValidation) error
	ListByNode(nodeID string, limit int) ([]ExpressVPNValidation, error)
	GetLatestForNode(nodeID string) (*ExpressVPNValidation, error)
}

type SystemMetricRepository interface {
	Create(metric *SystemMetricSnapshot) error
	List(limit int) ([]SystemMetricSnapshot, error)
}

type AgentCredentialRepository interface {
	Create(cred *AgentCredential) error
	Update(cred *AgentCredential) error
	GetByAgentID(agentID string) (*AgentCredential, error)
	Delete(agentID string) error
}

type AgentCommandRepository interface {
	Create(cmd *AgentCommand) error
	Update(cmd *AgentCommand) error
	GetByID(id string) (*AgentCommand, error)
	GetByIdempotencyKey(key string) (*AgentCommand, error)
	ListPendingByAgent(agentID string) ([]AgentCommand, error)
	ListPendingAll() ([]AgentCommand, error)
	ListActive() ([]AgentCommand, error)
	ListAll(limit int) ([]AgentCommand, error)
}
