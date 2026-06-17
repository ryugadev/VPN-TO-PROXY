package repository

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"vpn-to-proxy/internal/domain"
)

type sqliteRepo struct {
	db *gorm.DB
}

func NewSQLiteDB(dbPath string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// SQLite Performance Pragmas
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA busy_timeout=10000;")
	db.Exec("PRAGMA foreign_keys=ON;")

	// Configure connection pool
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}

	err = db.AutoMigrate(
		&domain.VPNNode{},
		&domain.Proxy{},
		&domain.HealthMetric{},
		&domain.AuditLog{},
		&domain.SystemConfig{},
		&domain.VPNCredential{},
		&domain.Agent{},
		&domain.ExpressVPNSession{},
		&domain.AgentHeartbeat{},
		&domain.PortAllocation{},
		&domain.RotationPolicy{},
		&domain.ExpressVPNValidation{},
		&domain.SystemMetricSnapshot{},
		&domain.AgentCredential{},
		&domain.AgentCommand{},
		&domain.Customer{},
		&domain.CustomerCredential{},
		&domain.CustomerSession{},
		&domain.CustomerApiKey{},
		&domain.ProxyPlan{},
		&domain.CustomerSubscription{},
		&domain.CustomerProxyCredential{},
		&domain.CustomerProxyAllocation{},
		&domain.UsageMetric{},
		&domain.Invoice{},
		&domain.PaymentProvider{},
		&domain.BillingEvent{},
		&domain.Plan{},
		&domain.PlanFeature{},
		&domain.Subscription{},
		&domain.InvoiceItem{},
		&domain.AuditEvent{},
		&domain.AbuseRule{},
		&domain.AbuseEvent{},
		&domain.CustomerRiskScore{},
		&domain.IPWhitelist{},
		&domain.BlockedTarget{},
		&domain.ConnectionLimit{},
		&domain.ProxyPool{},
		&domain.ProxyPoolMember{},
		&domain.ProxyQualitySnapshot{},
		&domain.StickySession{},
		&domain.RoutingPolicy{},
		&domain.ProxyReservation{},
		&domain.RoutingEvent{},
	)
	if err != nil {
		return nil, err
	}

	if err := verifySQLiteWritable(db); err != nil {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		return nil, err
	}

	return db, nil
}

func verifySQLiteWritable(db *gorm.DB) error {
	table := "__vpn_to_proxy_write_check"
	if err := db.Exec("CREATE TABLE IF NOT EXISTS " + table + " (id INTEGER PRIMARY KEY, checked_at INTEGER NOT NULL)").Error; err != nil {
		return fmt.Errorf("sqlite write check failed while creating table: %w", err)
	}
	if err := db.Exec("INSERT INTO "+table+" (checked_at) VALUES (?)", time.Now().UnixNano()).Error; err != nil {
		return fmt.Errorf("sqlite write check failed while inserting row: %w", err)
	}
	if err := db.Exec("DELETE FROM " + table).Error; err != nil {
		return fmt.Errorf("sqlite write check failed while cleaning table: %w", err)
	}
	return nil
}

type vpnNodeRepo struct {
	db *gorm.DB
}

func NewVPNNodeRepository(db *gorm.DB) domain.VPNNodeRepository {
	return &vpnNodeRepo{db: db}
}

func (r *vpnNodeRepo) Create(node *domain.VPNNode) error {
	return r.db.Create(node).Error
}

func (r *vpnNodeRepo) Update(node *domain.VPNNode) error {
	return r.db.Save(node).Error
}

func (r *vpnNodeRepo) Delete(id string) error {
	return r.db.Delete(&domain.VPNNode{}, "id = ?", id).Error
}

func (r *vpnNodeRepo) GetByID(id string) (*domain.VPNNode, error) {
	var node domain.VPNNode
	err := r.db.First(&node, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &node, nil
}

func (r *vpnNodeRepo) List() ([]domain.VPNNode, error) {
	var nodes []domain.VPNNode
	err := r.db.Find(&nodes).Error
	return nodes, err
}

type proxyRepo struct {
	db *gorm.DB
}

func NewProxyRepository(db *gorm.DB) domain.ProxyRepository {
	return &proxyRepo{db: db}
}

func (r *proxyRepo) Create(proxy *domain.Proxy) error {
	return r.db.Create(proxy).Error
}

func (r *proxyRepo) Update(proxy *domain.Proxy) error {
	return r.db.Save(proxy).Error
}

func (r *proxyRepo) Delete(id string) error {
	return r.db.Delete(&domain.Proxy{}, "id = ?", id).Error
}

func (r *proxyRepo) GetByID(id string) (*domain.Proxy, error) {
	var prxy domain.Proxy
	err := r.db.First(&prxy, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &prxy, nil
}

func (r *proxyRepo) GetByPort(port int) (*domain.Proxy, error) {
	var prxy domain.Proxy
	err := r.db.First(&prxy, "port = ?", port).Error
	if err != nil {
		return nil, err
	}
	return &prxy, nil
}

func (r *proxyRepo) List() ([]domain.Proxy, error) {
	var proxies []domain.Proxy
	err := r.db.Find(&proxies).Error
	return proxies, err
}

func (r *proxyRepo) GetByVPNNodeID(vpnNodeID string) ([]domain.Proxy, error) {
	var proxies []domain.Proxy
	err := r.db.Find(&proxies, "vpn_node_id = ?", vpnNodeID).Error
	return proxies, err
}

type auditLogRepo struct {
	db *gorm.DB
}

func NewAuditLogRepository(db *gorm.DB) domain.AuditLogRepository {
	return &auditLogRepo{db: db}
}

func (r *auditLogRepo) Create(log *domain.AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditLogRepo) List(limit int) ([]domain.AuditLog, error) {
	var logs []domain.AuditLog
	err := r.db.Order("timestamp desc").Limit(limit).Find(&logs).Error
	return logs, err
}

type healthMetricRepo struct {
	db *gorm.DB
}

func NewHealthMetricRepository(db *gorm.DB) domain.HealthMetricRepository {
	return &healthMetricRepo{db: db}
}

func (r *healthMetricRepo) Create(metric *domain.HealthMetric) error {
	return r.db.Create(metric).Error
}

func (r *healthMetricRepo) GetLatest(targetID string, limit int) ([]domain.HealthMetric, error) {
	var metrics []domain.HealthMetric
	err := r.db.Where("target_id = ?", targetID).Order("checked_at desc").Limit(limit).Find(&metrics).Error
	return metrics, err
}

type vpnCredentialRepo struct {
	db *gorm.DB
}

func NewVPNCredentialRepository(db *gorm.DB) domain.VPNCredentialRepository {
	return &vpnCredentialRepo{db: db}
}

func (r *vpnCredentialRepo) Create(cred *domain.VPNCredential) error {
	return r.db.Create(cred).Error
}

func (r *vpnCredentialRepo) Update(cred *domain.VPNCredential) error {
	return r.db.Save(cred).Error
}

func (r *vpnCredentialRepo) Delete(id string) error {
	return r.db.Delete(&domain.VPNCredential{}, "id = ?", id).Error
}

func (r *vpnCredentialRepo) GetByID(id string) (*domain.VPNCredential, error) {
	var cred domain.VPNCredential
	err := r.db.First(&cred, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

func (r *vpnCredentialRepo) List() ([]domain.VPNCredential, error) {
	var creds []domain.VPNCredential
	err := r.db.Find(&creds).Error
	return creds, err
}

type agentRepo struct {
	db *gorm.DB
}

func NewAgentRepository(db *gorm.DB) domain.AgentRepository {
	return &agentRepo{db: db}
}

func (r *agentRepo) Create(agent *domain.Agent) error {
	return r.db.Create(agent).Error
}

func (r *agentRepo) Update(agent *domain.Agent) error {
	return r.db.Save(agent).Error
}

func (r *agentRepo) Delete(id string) error {
	return r.db.Delete(&domain.Agent{}, "id = ?", id).Error
}

func (r *agentRepo) GetByID(id string) (*domain.Agent, error) {
	var agent domain.Agent
	err := r.db.First(&agent, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &agent, nil
}

func (r *agentRepo) List() ([]domain.Agent, error) {
	var agents []domain.Agent
	err := r.db.Find(&agents).Error
	return agents, err
}

type expressVPNSessionRepo struct {
	db *gorm.DB
}

func NewExpressVPNSessionRepository(db *gorm.DB) domain.ExpressVPNSessionRepository {
	return &expressVPNSessionRepo{db: db}
}

func (r *expressVPNSessionRepo) Create(session *domain.ExpressVPNSession) error {
	return r.db.Create(session).Error
}

func (r *expressVPNSessionRepo) Update(session *domain.ExpressVPNSession) error {
	return r.db.Save(session).Error
}

func (r *expressVPNSessionRepo) GetByID(id string) (*domain.ExpressVPNSession, error) {
	var session domain.ExpressVPNSession
	err := r.db.First(&session, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *expressVPNSessionRepo) GetActiveByAgent(agentID string) (*domain.ExpressVPNSession, error) {
	var session domain.ExpressVPNSession
	err := r.db.Where("agent_id = ? AND status = ?", agentID, "connected").First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *expressVPNSessionRepo) List() ([]domain.ExpressVPNSession, error) {
	var sessions []domain.ExpressVPNSession
	err := r.db.Find(&sessions).Error
	return sessions, err
}

// AgentHeartbeat
type agentHeartbeatRepo struct {
	db *gorm.DB
}

func NewAgentHeartbeatRepository(db *gorm.DB) domain.AgentHeartbeatRepository {
	return &agentHeartbeatRepo{db: db}
}

func (r *agentHeartbeatRepo) Create(hb *domain.AgentHeartbeat) error {
	return r.db.Create(hb).Error
}

func (r *agentHeartbeatRepo) ListByAgent(agentID string, limit int) ([]domain.AgentHeartbeat, error) {
	var hbs []domain.AgentHeartbeat
	err := r.db.Where("agent_id = ?", agentID).Order("timestamp desc").Limit(limit).Find(&hbs).Error
	return hbs, err
}

// PortAllocation
type portAllocationRepo struct {
	db *gorm.DB
}

func NewPortAllocationRepository(db *gorm.DB) domain.PortAllocationRepository {
	return &portAllocationRepo{db: db}
}

func (r *portAllocationRepo) Create(alloc *domain.PortAllocation) error {
	return r.db.Save(alloc).Error
}

func (r *portAllocationRepo) GetByPort(port int) (*domain.PortAllocation, error) {
	var alloc domain.PortAllocation
	err := r.db.First(&alloc, "port = ?", port).Error
	if err != nil {
		return nil, err
	}
	return &alloc, nil
}

func (r *portAllocationRepo) Delete(port int) error {
	return r.db.Delete(&domain.PortAllocation{}, "port = ?", port).Error
}

func (r *portAllocationRepo) List() ([]domain.PortAllocation, error) {
	var allocs []domain.PortAllocation
	err := r.db.Find(&allocs).Error
	return allocs, err
}

// RotationPolicy
type rotationPolicyRepo struct {
	db *gorm.DB
}

func NewRotationPolicyRepository(db *gorm.DB) domain.RotationPolicyRepository {
	return &rotationPolicyRepo{db: db}
}

func (r *rotationPolicyRepo) Create(policy *domain.RotationPolicy) error {
	return r.db.Create(policy).Error
}

func (r *rotationPolicyRepo) Update(policy *domain.RotationPolicy) error {
	return r.db.Save(policy).Error
}

func (r *rotationPolicyRepo) Delete(id string) error {
	return r.db.Delete(&domain.RotationPolicy{}, "id = ?", id).Error
}

func (r *rotationPolicyRepo) GetByID(id string) (*domain.RotationPolicy, error) {
	var policy domain.RotationPolicy
	err := r.db.First(&policy, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func (r *rotationPolicyRepo) List() ([]domain.RotationPolicy, error) {
	var policies []domain.RotationPolicy
	err := r.db.Find(&policies).Error
	return policies, err
}

// ExpressVPNValidation
type expressVPNValidationRepo struct {
	db *gorm.DB
}

func NewExpressVPNValidationRepository(db *gorm.DB) domain.ExpressVPNValidationRepository {
	return &expressVPNValidationRepo{db: db}
}

func (r *expressVPNValidationRepo) Create(report *domain.ExpressVPNValidation) error {
	return r.db.Create(report).Error
}

func (r *expressVPNValidationRepo) ListByNode(nodeID string, limit int) ([]domain.ExpressVPNValidation, error) {
	var reports []domain.ExpressVPNValidation
	err := r.db.Where("node_id = ?", nodeID).Order("timestamp desc").Limit(limit).Find(&reports).Error
	return reports, err
}

func (r *expressVPNValidationRepo) GetLatestForNode(nodeID string) (*domain.ExpressVPNValidation, error) {
	var report domain.ExpressVPNValidation
	err := r.db.Where("node_id = ?", nodeID).Order("timestamp desc").First(&report).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// SystemMetricSnapshot
type systemMetricRepo struct {
	db *gorm.DB
}

func NewSystemMetricRepository(db *gorm.DB) domain.SystemMetricRepository {
	return &systemMetricRepo{db: db}
}

func (r *systemMetricRepo) Create(metric *domain.SystemMetricSnapshot) error {
	return r.db.Create(metric).Error
}

func (r *systemMetricRepo) List(limit int) ([]domain.SystemMetricSnapshot, error) {
	var metrics []domain.SystemMetricSnapshot
	err := r.db.Order("timestamp desc").Limit(limit).Find(&metrics).Error
	return metrics, err
}

// AgentCredentialRepository implementation
type agentCredentialRepo struct {
	db *gorm.DB
}

func NewAgentCredentialRepository(db *gorm.DB) domain.AgentCredentialRepository {
	return &agentCredentialRepo{db: db}
}

func (r *agentCredentialRepo) Create(cred *domain.AgentCredential) error {
	return r.db.Create(cred).Error
}

func (r *agentCredentialRepo) Update(cred *domain.AgentCredential) error {
	return r.db.Save(cred).Error
}

func (r *agentCredentialRepo) GetByAgentID(agentID string) (*domain.AgentCredential, error) {
	var cred domain.AgentCredential
	err := r.db.Where("agent_id = ?", agentID).First(&cred).Error
	if err != nil {
		return nil, err
	}
	return &cred, nil
}

func (r *agentCredentialRepo) Delete(agentID string) error {
	return r.db.Where("agent_id = ?", agentID).Delete(&domain.AgentCredential{}).Error
}

// AgentCommandRepository implementation
type agentCommandRepo struct {
	db *gorm.DB
}

func NewAgentCommandRepository(db *gorm.DB) domain.AgentCommandRepository {
	return &agentCommandRepo{db: db}
}

func (r *agentCommandRepo) Create(cmd *domain.AgentCommand) error {
	return r.db.Create(cmd).Error
}

func (r *agentCommandRepo) Update(cmd *domain.AgentCommand) error {
	return r.db.Save(cmd).Error
}

func (r *agentCommandRepo) GetByID(id string) (*domain.AgentCommand, error) {
	var cmd domain.AgentCommand
	err := r.db.Where("id = ?", id).First(&cmd).Error
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

func (r *agentCommandRepo) GetByIdempotencyKey(key string) (*domain.AgentCommand, error) {
	var cmd domain.AgentCommand
	err := r.db.Where("idempotency_key = ?", key).First(&cmd).Error
	if err != nil {
		return nil, err
	}
	return &cmd, nil
}

func (r *agentCommandRepo) ListPendingByAgent(agentID string) ([]domain.AgentCommand, error) {
	var cmds []domain.AgentCommand
	err := r.db.Where("agent_id = ? AND status = ?", agentID, "pending").Order("created_at asc").Find(&cmds).Error
	return cmds, err
}

func (r *agentCommandRepo) ListPendingAll() ([]domain.AgentCommand, error) {
	var cmds []domain.AgentCommand
	err := r.db.Where("status = ?", "pending").Order("created_at asc").Find(&cmds).Error
	return cmds, err
}

func (r *agentCommandRepo) ListActive() ([]domain.AgentCommand, error) {
	var cmds []domain.AgentCommand
	err := r.db.Where("status = ? OR status = ?", "dispatched", "executing").Find(&cmds).Error
	return cmds, err
}

func (r *agentCommandRepo) ListAll(limit int) ([]domain.AgentCommand, error) {
	var cmds []domain.AgentCommand
	err := r.db.Order("created_at desc").Limit(limit).Find(&cmds).Error
	return cmds, err
}
