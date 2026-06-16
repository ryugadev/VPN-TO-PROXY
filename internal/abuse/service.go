package abuse

import (
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/proxy"
)

type Service struct {
	db *gorm.DB

	mu             sync.Mutex
	activeCustomer map[string]int
	activeProxy    map[string]int
	failedAuth     map[string][]time.Time
	requests       map[string][]time.Time
	tempBlocked    map[string]time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{
		db:             db,
		activeCustomer: make(map[string]int),
		activeProxy:    make(map[string]int),
		failedAuth:     make(map[string][]time.Time),
		requests:       make(map[string][]time.Time),
		tempBlocked:    make(map[string]time.Time),
	}
}

func (s *Service) EnsureDefaultRules() {
	defaults := []domain.AbuseRule{
		{ID: "failed-auth-warn", Name: "Failed auth warning", Type: "too_many_failed_auth", Threshold: 5, Action: "warn", Enabled: true},
		{ID: "failed-auth-block", Name: "Failed auth temporary block", Type: "too_many_failed_auth", Threshold: 20, Action: "throttle", Enabled: true},
		{ID: "failed-auth-suspend", Name: "Failed auth credential suspend", Type: "too_many_failed_auth", Threshold: 50, Action: "suspend_proxy", Enabled: true},
		{ID: "blocked-target", Name: "Blocked target attempt", Type: "blacklisted_target", Threshold: 1, Action: "warn", Enabled: true},
		{ID: "api-rate-limit", Name: "API rate limit", Type: "api_rate_limit", Threshold: 120, Action: "throttle", Enabled: true},
		{ID: "connection-limit", Name: "Connection limit", Type: "too_many_connections", Threshold: 1, Action: "throttle", Enabled: true},
	}
	for _, rule := range defaults {
		var existing domain.AbuseRule
		if err := s.db.First(&existing, "id = ?", rule.ID).Error; err == nil {
			continue
		}
		now := time.Now()
		rule.CreatedAt = now
		rule.UpdatedAt = now
		_ = s.db.Create(&rule).Error
	}
}

func (s *Service) ValidateProxyCredential(proxyID, username, password, clientIP string) proxy.AuthResult {
	key := proxyID + "|" + username + "|" + clientIP
	if until, blocked := s.blockedUntil(key); blocked {
		return proxy.AuthResult{Allowed: false, Reason: "temporarily blocked until " + until.Format(time.RFC3339)}
	}

	var cred domain.CustomerProxyCredential
	if err := s.db.First(&cred, "proxy_id = ? AND username = ? AND status = ?", proxyID, username, "active").Error; err != nil {
		s.RecordFailedAuth("", proxyID, username, clientIP)
		return proxy.AuthResult{Allowed: false, Reason: "invalid credentials"}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)); err != nil {
		s.RecordFailedAuth(cred.CustomerID, proxyID, username, clientIP)
		return proxy.AuthResult{Allowed: false, CustomerID: cred.CustomerID, Reason: "invalid credentials"}
	}
	if !s.clientIPAllowed(cred.CustomerID, clientIP) {
		s.createEvent(cred.CustomerID, proxyID, "", "medium", "proxy denied by IP whitelist", map[string]interface{}{"client_ip": clientIP})
		s.bumpRisk(cred.CustomerID, 8, "ip_whitelist_denied")
		return proxy.AuthResult{Allowed: false, CustomerID: cred.CustomerID, Reason: "client IP is not whitelisted"}
	}
	if err := s.checkRequestRate(cred.CustomerID, "proxy-auth", 120, time.Minute); err != nil {
		s.createEvent(cred.CustomerID, proxyID, "api-rate-limit", "medium", "proxy auth request rate exceeded", nil)
		return proxy.AuthResult{Allowed: false, CustomerID: cred.CustomerID, Reason: err.Error()}
	}
	return proxy.AuthResult{Allowed: true, CustomerID: cred.CustomerID}
}

func (s *Service) GuardConnection(proxyID, customerID, clientIP, target string) (func(), error) {
	if customerID == "" {
		return func() {}, nil
	}
	if err := s.checkBlockedTarget(customerID, proxyID, target); err != nil {
		return nil, err
	}
	if err := s.checkConnectionLimits(customerID, proxyID); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.activeCustomer[customerID]++
	s.activeProxy[proxyID]++
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		if s.activeCustomer[customerID] > 0 {
			s.activeCustomer[customerID]--
		}
		if s.activeProxy[proxyID] > 0 {
			s.activeProxy[proxyID]--
		}
		s.mu.Unlock()
	}, nil
}

func (s *Service) RecordUsage(customerID, proxyID string, inBytes, outBytes uint64) {
	if customerID == "" || proxyID == "" {
		return
	}
	now := time.Now()
	_ = s.db.Create(&domain.UsageMetric{CustomerID: customerID, ProxyID: proxyID, BandwidthIn: inBytes, BandwidthOut: outBytes, RequestCount: 1, Bucket: "hourly", PeriodStart: now.Truncate(time.Hour), CreatedAt: now}).Error
	_ = s.db.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND proxy_id = ? AND status <> ?", customerID, proxyID, "deleted").Updates(map[string]interface{}{
		"bandwidth_in":  gorm.Expr("bandwidth_in + ?", inBytes),
		"bandwidth_out": gorm.Expr("bandwidth_out + ?", outBytes),
		"updated_at":    now,
	}).Error
	s.detectBandwidthAbuse(customerID, proxyID, inBytes+outBytes)
}

func (s *Service) RecordFailedAuth(customerID, proxyID, username, clientIP string) {
	key := proxyID + "|" + username + "|" + clientIP
	now := time.Now()
	s.mu.Lock()
	s.failedAuth[key] = pruneTimes(append(s.failedAuth[key], now), now.Add(-time.Hour))
	count5 := countSince(s.failedAuth[key], now.Add(-5*time.Minute))
	count10 := countSince(s.failedAuth[key], now.Add(-10*time.Minute))
	count60 := len(s.failedAuth[key])
	s.mu.Unlock()

	if count5 >= 5 {
		s.createEvent(customerID, proxyID, "failed-auth-warn", "low", "5 failed auth attempts in 5 minutes", map[string]interface{}{"username": username, "client_ip": clientIP})
		s.bumpRisk(customerID, 3, "failed_auth_warning")
	}
	if count10 >= 20 {
		s.mu.Lock()
		s.tempBlocked[key] = now.Add(10 * time.Minute)
		s.mu.Unlock()
		s.createEvent(customerID, proxyID, "failed-auth-block", "high", "20 failed auth attempts in 10 minutes", map[string]interface{}{"username": username, "client_ip": clientIP})
		s.bumpRisk(customerID, 12, "failed_auth_temporary_block")
	}
	if count60 >= 50 {
		_ = s.db.Model(&domain.CustomerProxyCredential{}).Where("proxy_id = ? AND username = ?", proxyID, username).Update("status", "suspended").Error
		s.createEvent(customerID, proxyID, "failed-auth-suspend", "critical", "50 failed auth attempts in 1 hour; credential suspended", map[string]interface{}{"username": username, "client_ip": clientIP})
		s.bumpRisk(customerID, 30, "credential_suspended")
	}
}

func (s *Service) CheckAPIRate(identity, scope string, limit int, window time.Duration) bool {
	return s.checkRequestRate(identity, scope, limit, window) == nil
}

func (s *Service) ListWhitelist(customerID string) ([]domain.IPWhitelist, error) {
	var rows []domain.IPWhitelist
	err := s.db.Where("customer_id = ?", customerID).Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) AddWhitelist(customerID, ip, cidr, description string) (*domain.IPWhitelist, error) {
	row := &domain.IPWhitelist{ID: uuid.New().String(), CustomerID: customerID, IPAddress: ip, CIDR: cidr, Description: description, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	err := s.db.Create(row).Error
	if err == nil {
		s.audit(customerID, "customer", "WHITELIST_CHANGED", row.ID, "ip_whitelist", map[string]interface{}{"ip": ip, "cidr": cidr})
	}
	return row, err
}

func (s *Service) DeleteWhitelist(customerID, id string) error {
	err := s.db.Delete(&domain.IPWhitelist{}, "id = ? AND customer_id = ?", id, customerID).Error
	if err == nil {
		s.audit(customerID, "customer", "WHITELIST_CHANGED", id, "ip_whitelist", map[string]interface{}{"deleted": true})
	}
	return err
}

func (s *Service) ListBlockedTargets() ([]domain.BlockedTarget, error) {
	var rows []domain.BlockedTarget
	err := s.db.Order("created_at desc").Find(&rows).Error
	return rows, err
}

func (s *Service) AddBlockedTarget(t, value, reason string) (*domain.BlockedTarget, error) {
	row := &domain.BlockedTarget{ID: uuid.New().String(), Type: t, Value: value, Reason: reason, Enabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	err := s.db.Create(row).Error
	if err == nil {
		s.audit("admin", "system", "BLOCKED_TARGET_CHANGED", row.ID, "blocked_target", map[string]interface{}{"type": t, "value": value})
	}
	return row, err
}

func (s *Service) DeleteBlockedTarget(id string) error {
	err := s.db.Delete(&domain.BlockedTarget{}, "id = ?", id).Error
	if err == nil {
		s.audit("admin", "system", "BLOCKED_TARGET_CHANGED", id, "blocked_target", map[string]interface{}{"deleted": true})
	}
	return err
}

func (s *Service) RecentEvents(limit int) ([]domain.AbuseEvent, error) {
	var rows []domain.AbuseEvent
	err := s.db.Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) HighRiskCustomers(limit int) ([]domain.CustomerRiskScore, error) {
	var rows []domain.CustomerRiskScore
	err := s.db.Order("score desc").Limit(limit).Find(&rows).Error
	return rows, err
}

func (s *Service) Dashboard() (map[string]interface{}, error) {
	events, _ := s.RecentEvents(100)
	risks, _ := s.HighRiskCustomers(50)
	var suspendedProxies, failedAuth, blockedAttempts, bandwidthSpikes int64
	s.db.Model(&domain.CustomerProxyAllocation{}).Where("status = ?", "suspended").Count(&suspendedProxies)
	s.db.Model(&domain.AbuseEvent{}).Where("rule_id IN ?", []string{"failed-auth-warn", "failed-auth-block", "failed-auth-suspend"}).Count(&failedAuth)
	s.db.Model(&domain.AbuseEvent{}).Where("rule_id = ?", "blocked-target").Count(&blockedAttempts)
	s.db.Model(&domain.AbuseEvent{}).Where("rule_id = ?", "bandwidth-spike").Count(&bandwidthSpikes)
	return map[string]interface{}{
		"events":                  events,
		"high_risk_customers":     risks,
		"suspended_proxies":       suspendedProxies,
		"failed_auth_attempts":    failedAuth,
		"blocked_target_attempts": blockedAttempts,
		"bandwidth_spikes":        bandwidthSpikes,
	}, nil
}

func (s *Service) SuspendCustomer(customerID string) error {
	now := time.Now()
	err := s.db.Model(&domain.Customer{}).Where("id = ?", customerID).Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error
	if err == nil {
		_ = s.db.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND status = ?", customerID, "active").Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error
		s.audit("admin", "system", "CUSTOMER_SUSPENDED", customerID, "customer", nil)
	}
	return err
}

func (s *Service) UnsuspendCustomer(customerID string) error {
	err := s.db.Model(&domain.Customer{}).Where("id = ?", customerID).Updates(map[string]interface{}{"status": "active", "updated_at": time.Now()}).Error
	if err == nil {
		s.audit("admin", "system", "CUSTOMER_UNSUSPENDED", customerID, "customer", nil)
	}
	return err
}

func (s *Service) SuspendProxy(allocationID string) error {
	err := s.db.Model(&domain.CustomerProxyAllocation{}).Where("id = ?", allocationID).Update("status", "suspended").Error
	if err == nil {
		s.audit("admin", "system", "PROXY_SUSPENDED", allocationID, "proxy_allocation", nil)
	}
	return err
}

func (s *Service) DisableCredential(credentialID string) error {
	err := s.db.Model(&domain.CustomerProxyCredential{}).Where("id = ?", credentialID).Update("status", "suspended").Error
	if err == nil {
		s.audit("admin", "system", "CREDENTIAL_DISABLED", credentialID, "proxy_credential", nil)
	}
	return err
}

func (s *Service) ClearRisk(customerID string) error {
	err := s.db.Save(&domain.CustomerRiskScore{CustomerID: customerID, Score: 0, Level: "low", Factors: "[]", UpdatedAt: time.Now()}).Error
	if err == nil {
		s.audit("admin", "system", "RISK_SCORE_CHANGED", customerID, "customer", map[string]interface{}{"score": 0})
	}
	return err
}

func (s *Service) clientIPAllowed(customerID, clientIP string) bool {
	var rows []domain.IPWhitelist
	if err := s.db.Where("customer_id = ? AND enabled = ?", customerID, true).Find(&rows).Error; err != nil || len(rows) == 0 {
		return true
	}
	ip := net.ParseIP(stripPort(clientIP))
	if ip == nil {
		return false
	}
	for _, row := range rows {
		if row.IPAddress != "" && net.ParseIP(row.IPAddress).Equal(ip) {
			return true
		}
		if row.CIDR != "" {
			_, network, err := net.ParseCIDR(row.CIDR)
			if err == nil && network.Contains(ip) {
				return true
			}
		}
	}
	return false
}

func (s *Service) checkConnectionLimits(customerID, proxyID string) error {
	limit := s.planConcurrentLimit(customerID)
	maxPerProxy := 0
	var custom domain.ConnectionLimit
	if err := s.db.First(&custom, "customer_id = ? OR proxy_id = ?", customerID, proxyID).Error; err == nil {
		if custom.MaxConcurrent > 0 {
			limit = custom.MaxConcurrent
		}
		maxPerProxy = custom.MaxPerProxy
	}
	s.mu.Lock()
	customerActive := s.activeCustomer[customerID]
	proxyActive := s.activeProxy[proxyID]
	s.mu.Unlock()
	if limit > 0 && customerActive >= limit {
		s.createEvent(customerID, proxyID, "connection-limit", "high", "customer concurrent connection limit exceeded", map[string]interface{}{"limit": limit})
		s.bumpRisk(customerID, 10, "connection_limit")
		return errors.New("customer connection limit exceeded")
	}
	if maxPerProxy > 0 && proxyActive >= maxPerProxy {
		s.createEvent(customerID, proxyID, "connection-limit", "medium", "proxy concurrent connection limit exceeded", map[string]interface{}{"limit": maxPerProxy})
		return errors.New("proxy connection limit exceeded")
	}
	return nil
}

func (s *Service) checkBlockedTarget(customerID, proxyID, target string) error {
	host, port, _ := strings.Cut(target, ":")
	var rows []domain.BlockedTarget
	if err := s.db.Where("enabled = ?", true).Find(&rows).Error; err != nil {
		return nil
	}
	for _, row := range rows {
		match := false
		switch row.Type {
		case "domain":
			match = strings.EqualFold(host, row.Value) || strings.HasSuffix(strings.ToLower(host), "."+strings.ToLower(row.Value))
		case "ip":
			match = net.ParseIP(host) != nil && net.ParseIP(host).Equal(net.ParseIP(row.Value))
		case "cidr":
			_, network, err := net.ParseCIDR(row.Value)
			match = err == nil && net.ParseIP(host) != nil && network.Contains(net.ParseIP(host))
		case "port":
			match = port == row.Value
		}
		if match {
			s.createEvent(customerID, proxyID, "blocked-target", "high", "blocked target rejected", map[string]interface{}{"target": target, "blocked_target": row.Value, "type": row.Type})
			s.bumpRisk(customerID, 15, "blocked_target")
			return errors.New("target blocked by policy")
		}
	}
	return nil
}

func (s *Service) checkRequestRate(identity, scope string, limit int, window time.Duration) error {
	if identity == "" {
		identity = "anonymous"
	}
	key := scope + "|" + identity
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[key] = pruneTimes(append(s.requests[key], now), now.Add(-window))
	if len(s.requests[key]) > limit {
		return errors.New("rate limit exceeded")
	}
	return nil
}

func (s *Service) planConcurrentLimit(customerID string) int {
	var sub domain.Subscription
	if err := s.db.Where("customer_id = ? AND status = ? AND expires_at > ?", customerID, "active", time.Now()).Order("expires_at desc").First(&sub).Error; err != nil {
		return 0
	}
	var plan domain.Plan
	if err := s.db.First(&plan, "id = ?", sub.PlanID).Error; err != nil {
		return 0
	}
	return plan.ConcurrentConnections
}

func (s *Service) detectBandwidthAbuse(customerID, proxyID string, bytes uint64) {
	if bytes > 512*1024*1024 {
		s.createEvent(customerID, proxyID, "bandwidth-spike", "medium", "large single-connection bandwidth spike", map[string]interface{}{"bytes": bytes})
		s.bumpRisk(customerID, 10, "bandwidth_spike")
	}
	var sub domain.Subscription
	if err := s.db.Where("customer_id = ? AND status = ?", customerID, "active").Order("expires_at desc").First(&sub).Error; err != nil {
		return
	}
	var plan domain.Plan
	if err := s.db.First(&plan, "id = ?", sub.PlanID).Error; err != nil || plan.BandwidthLimitGB <= 0 {
		return
	}
	var usage struct {
		BandwidthInTotal  uint64
		BandwidthOutTotal uint64
	}
	s.db.Model(&domain.CustomerProxyAllocation{}).
		Select("coalesce(sum(bandwidth_in),0) as bandwidth_in_total, coalesce(sum(bandwidth_out),0) as bandwidth_out_total").
		Where("customer_id = ?", customerID).
		Scan(&usage)
	if usage.BandwidthInTotal+usage.BandwidthOutTotal > uint64(plan.BandwidthLimitGB)*1024*1024*1024 {
		s.createEvent(customerID, proxyID, "bandwidth-limit", "critical", "bandwidth limit exceeded", nil)
		s.bumpRisk(customerID, 20, "bandwidth_limit")
	}
}

func (s *Service) createEvent(customerID, proxyID, ruleID, severity, message string, metadata map[string]interface{}) {
	raw, _ := json.Marshal(metadata)
	_ = s.db.Create(&domain.AbuseEvent{ID: uuid.New().String(), CustomerID: customerID, ProxyID: proxyID, RuleID: ruleID, Severity: severity, Message: message, Metadata: string(raw), CreatedAt: time.Now()}).Error
	s.audit("system", "enforcement", "PROXY_DENIED", proxyID, "proxy", map[string]interface{}{"customer_id": customerID, "rule_id": ruleID, "severity": severity, "message": message})
}

func (s *Service) bumpRisk(customerID string, delta int, factor string) {
	if customerID == "" {
		return
	}
	var score domain.CustomerRiskScore
	if err := s.db.First(&score, "customer_id = ?", customerID).Error; err != nil {
		score = domain.CustomerRiskScore{CustomerID: customerID, Score: 0, Level: "low", Factors: "[]"}
	}
	score.Score += delta
	if score.Score > 100 {
		score.Score = 100
	}
	score.Level = riskLevel(score.Score)
	score.Factors = appendJSONFactor(score.Factors, factor)
	score.UpdatedAt = time.Now()
	_ = s.db.Save(&score).Error
	s.audit("system", "risk", "RISK_SCORE_CHANGED", customerID, "customer", map[string]interface{}{"score": score.Score, "level": score.Level, "factor": factor})
}

func (s *Service) audit(actorID, actorType, action, targetID, targetType string, metadata map[string]interface{}) {
	raw, _ := json.Marshal(metadata)
	_ = s.db.Create(&domain.AuditEvent{ID: uuid.New().String(), ActorID: actorID, ActorType: actorType, Action: action, TargetID: targetID, TargetType: targetType, Metadata: string(raw), CreatedAt: time.Now()}).Error
}

func (s *Service) blockedUntil(key string) (time.Time, bool) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	until, ok := s.tempBlocked[key]
	if !ok || until.Before(now) {
		delete(s.tempBlocked, key)
		return time.Time{}, false
	}
	return until, true
}

func pruneTimes(values []time.Time, cutoff time.Time) []time.Time {
	out := values[:0]
	for _, v := range values {
		if v.After(cutoff) {
			out = append(out, v)
		}
	}
	return out
}

func countSince(values []time.Time, cutoff time.Time) int {
	n := 0
	for _, v := range values {
		if v.After(cutoff) {
			n++
		}
	}
	return n
}

func stripPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}

func riskLevel(score int) string {
	switch {
	case score >= 80:
		return "critical"
	case score >= 60:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

func appendJSONFactor(raw, factor string) string {
	var factors []string
	_ = json.Unmarshal([]byte(raw), &factors)
	factors = append(factors, factor)
	b, _ := json.Marshal(factors)
	return string(b)
}

func targetPort(target string) int {
	_, port, _ := strings.Cut(target, ":")
	n, _ := strconv.Atoi(port)
	return n
}
