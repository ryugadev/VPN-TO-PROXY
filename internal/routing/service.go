package routing

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
)

type Service struct {
	db *gorm.DB
	rr map[string]int
}

type Candidate struct {
	Proxy       domain.Proxy                `json:"proxy"`
	Pool        domain.ProxyPool            `json:"pool"`
	Quality     domain.ProxyQualitySnapshot `json:"quality"`
	Agent       domain.Agent                `json:"agent"`
	VPNNode     domain.VPNNode              `json:"vpn_node"`
	ActiveLoad  int                         `json:"active_load"`
	Weighted    int                         `json:"weighted_score"`
	Reservation *domain.ProxyReservation    `json:"reservation,omitempty"`
	Metadata    map[string]interface{}      `json:"metadata,omitempty"`
}

type PoolOverview struct {
	Pool              domain.ProxyPool         `json:"pool"`
	AvailableProxies  int                      `json:"available_proxies"`
	PoolSize          int                      `json:"pool_size"`
	ActiveSessions    int                      `json:"active_sessions"`
	AverageQuality    int                      `json:"average_quality"`
	Health            string                   `json:"health"`
	AgentRedundancy   int                      `json:"agent_redundancy"`
	CountryRedundancy int                      `json:"country_redundancy"`
	Members           []domain.ProxyPoolMember `json:"members,omitempty"`
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, rr: make(map[string]int)}
}

func (s *Service) EnsureDefaultPools() {
	for _, country := range []string{"Vietnam", "Singapore", "Japan", "USA", "Germany", "UK"} {
		_ = s.CreatePool(country, "", "weighted_score")
	}
	_ = s.SyncPools()
}

func (s *Service) CreatePool(country, region, strategy string) error {
	if strings.TrimSpace(country) == "" {
		return errors.New("country is required")
	}
	if strategy == "" {
		strategy = "weighted_score"
	}
	now := time.Now()
	var existing domain.ProxyPool
	if err := s.db.First(&existing, "country = ?", country).Error; err == nil {
		return s.db.Model(&existing).Updates(map[string]interface{}{
			"region":     region,
			"strategy":   strategy,
			"status":     "active",
			"updated_at": now,
		}).Error
	}
	return s.db.Create(&domain.ProxyPool{ID: uuid.New().String(), Country: country, Region: region, Status: "active", Strategy: strategy, CreatedAt: now, UpdatedAt: now}).Error
}

func (s *Service) SyncPools() error {
	var proxies []domain.Proxy
	if err := s.db.Find(&proxies).Error; err != nil {
		return err
	}
	now := time.Now()
	for _, p := range proxies {
		country := p.Country
		if country == "" {
			country = countryFromVPNNode(s.db, p.VPNNodeID)
		}
		if country == "" {
			country = "Unassigned"
		}
		if err := s.CreatePool(country, p.Region, "weighted_score"); err != nil {
			return err
		}
		var pool domain.ProxyPool
		if err := s.db.First(&pool, "country = ?", country).Error; err != nil {
			return err
		}
		quality := s.CalculateQuality(p.ID)
		load := s.activeLoad(p.ID)
		healthStatus := quality.HealthStatus
		if healthStatus == "" {
			healthStatus = "unknown"
		}
		member := domain.ProxyPoolMember{
			ID:           uuid.New().String(),
			PoolID:       pool.ID,
			ProxyID:      p.ID,
			AgentID:      p.AgentID,
			VPNNodeID:    p.VPNNodeID,
			HealthStatus: healthStatus,
			QualityScore: quality.Score,
			ActiveLoad:   load,
			Status:       memberStatus(p, quality),
			LastChecked:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		var existing domain.ProxyPoolMember
		if err := s.db.First(&existing, "proxy_id = ?", p.ID).Error; err == nil {
			member.ID = existing.ID
			member.CreatedAt = existing.CreatedAt
			_ = s.db.Model(&existing).Updates(member).Error
			continue
		}
		_ = s.db.Create(&member).Error
	}
	return nil
}

func (s *Service) CalculateQuality(proxyID string) domain.ProxyQualitySnapshot {
	var p domain.Proxy
	if err := s.db.First(&p, "id = ?", proxyID).Error; err != nil {
		return domain.ProxyQualitySnapshot{ProxyID: proxyID, Score: 0, Grade: "Dead", HealthStatus: "missing", CreatedAt: time.Now()}
	}
	var metrics []domain.HealthMetric
	_ = s.db.Where("target_id = ?", proxyID).Order("checked_at desc").Limit(20).Find(&metrics).Error
	latency := int64(300)
	online := 0
	failures := 0
	if len(metrics) > 0 {
		var total int64
		for _, m := range metrics {
			if m.LatencyMs > 0 {
				total += m.LatencyMs
			}
			if m.Status == "online" || m.Status == "healthy" {
				online++
			} else {
				failures++
			}
		}
		latency = total / int64(len(metrics))
	} else if p.Status == "running" {
		online = 1
	}
	uptime := 100.0
	success := 100.0
	if len(metrics) > 0 {
		uptime = float64(online) / float64(len(metrics)) * 100
		success = uptime
	}
	score := 100
	score -= int(math.Min(float64(latency)/10, 30))
	score -= failures * 8
	if p.Status == "error" || p.Status == "stopped" {
		score -= 45
	}
	if agentOffline(s.db, p.AgentID) {
		score -= 35
	}
	load := s.activeLoad(proxyID)
	score -= load * 3
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	health := "healthy"
	if score == 0 || p.Status == "error" {
		health = "dead"
	} else if score < 50 {
		health = "degraded"
	}
	snapshot := domain.ProxyQualitySnapshot{
		ID:                    uuid.New().String(),
		ProxyID:               proxyID,
		Score:                 score,
		Grade:                 Grade(score),
		LatencyMs:             latency,
		UptimePercent:         uptime,
		ConnectionSuccessRate: success,
		BandwidthAvailableGB:  0,
		RecentFailures:        failures,
		HealthStatus:          health,
		CreatedAt:             time.Now(),
	}
	_ = s.db.Create(&snapshot).Error
	return snapshot
}

func Grade(score int) string {
	switch {
	case score >= 95:
		return "A+"
	case score >= 85:
		return "A"
	case score >= 70:
		return "B"
	case score >= 40:
		return "C"
	default:
		return "Dead"
	}
}

func (s *Service) SelectProxy(input domain.RoutingSelectionInput) (*Candidate, error) {
	if err := s.SyncPools(); err != nil {
		return nil, err
	}
	if sticky := s.findSticky(input); sticky != nil {
		candidate, err := s.candidateForProxy(sticky.ProxyID, input)
		if err == nil && candidate.Quality.Grade != "Dead" {
			s.recordEvent(input.CustomerID, candidate.Proxy.ID, candidate.Pool.ID, "ProxySelected", "sticky session proxy selected", map[string]interface{}{"session_key": input.SessionKey})
			return candidate, nil
		}
	}
	candidates, err := s.candidates(input)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, errors.New("no healthy proxy available for routing policy")
	}
	chosen := s.balance(input, candidates)
	if input.SessionKey != "" && isSticky(input.RotationMode) {
		s.saveSticky(input, chosen.Proxy.ID)
	}
	s.recordEvent(input.CustomerID, chosen.Proxy.ID, chosen.Pool.ID, "ProxySelected", "smart routing selected proxy", map[string]interface{}{"country": input.Country, "strategy": strategy(input.Strategy)})
	return chosen, nil
}

func (s *Service) SelectDomainProxy(input domain.RoutingSelectionInput) (*domain.Proxy, error) {
	candidate, err := s.SelectProxy(input)
	if err != nil {
		return nil, err
	}
	return &candidate.Proxy, nil
}

func (s *Service) RotateAllocation(customerID, allocationID, reason string) (*Candidate, error) {
	var alloc domain.CustomerProxyAllocation
	if err := s.db.First(&alloc, "id = ? AND customer_id = ?", allocationID, customerID).Error; err != nil {
		return nil, err
	}
	input := domain.RoutingSelectionInput{CustomerID: customerID, Country: alloc.Country, RotationMode: alloc.RotationMode}
	candidates, err := s.candidates(input)
	if err != nil {
		return nil, err
	}
	for _, c := range candidates {
		if c.Proxy.ID == alloc.ProxyID {
			continue
		}
		if err := s.db.Model(&alloc).Updates(map[string]interface{}{"proxy_id": c.Proxy.ID, "updated_at": time.Now()}).Error; err != nil {
			return nil, err
		}
		s.recordEvent(customerID, c.Proxy.ID, c.Pool.ID, "ProxyRotated", "proxy allocation rotated", map[string]interface{}{"allocation_id": allocationID, "reason": reason, "previous_proxy_id": alloc.ProxyID})
		return c, nil
	}
	return nil, errors.New("no replacement proxy available")
}

func (s *Service) FailoverAllocation(customerID, allocationID string) (*Candidate, error) {
	c, err := s.RotateAllocation(customerID, allocationID, "failover")
	if err == nil {
		s.recordEvent(customerID, c.Proxy.ID, c.Pool.ID, "ProxyFailedOver", "proxy allocation failed over", map[string]interface{}{"allocation_id": allocationID})
	}
	return c, err
}

func (s *Service) CreateReservation(r *domain.ProxyReservation) error {
	now := time.Now()
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	if r.Status == "" {
		r.Status = "active"
	}
	if r.StartsAt.IsZero() {
		r.StartsAt = now
	}
	r.CreatedAt = now
	r.UpdatedAt = now
	return s.db.Create(r).Error
}

func (s *Service) ListPools() ([]PoolOverview, error) {
	if err := s.SyncPools(); err != nil {
		return nil, err
	}
	var pools []domain.ProxyPool
	if err := s.db.Order("country asc").Find(&pools).Error; err != nil {
		return nil, err
	}
	out := make([]PoolOverview, 0, len(pools))
	for _, pool := range pools {
		overview, _ := s.PoolOverview(pool.Country, true)
		out = append(out, overview)
	}
	return out, nil
}

func (s *Service) PoolOverview(country string, includeMembers bool) (PoolOverview, error) {
	var pool domain.ProxyPool
	if err := s.db.First(&pool, "country = ?", country).Error; err != nil {
		return PoolOverview{}, err
	}
	var members []domain.ProxyPoolMember
	s.db.Where("pool_id = ?", pool.ID).Order("quality_score desc").Find(&members)
	agents := map[string]bool{}
	total := 0
	available := 0
	active := 0
	for _, m := range members {
		total += m.QualityScore
		active += m.ActiveLoad
		if m.AgentID != "" {
			agents[m.AgentID] = true
		}
		if m.Status == "active" && m.HealthStatus != "dead" && m.QualityScore >= 40 {
			available++
		}
	}
	avg := 0
	if len(members) > 0 {
		avg = total / len(members)
	}
	health := "healthy"
	if available == 0 {
		health = "dead"
		s.recordEvent("", "", pool.ID, "PoolDegraded", "pool has no available proxies", map[string]interface{}{"country": country})
	} else if len(agents) < 2 {
		health = "degraded"
		s.recordEvent("", "", pool.ID, "PoolDegraded", "pool has low agent redundancy", map[string]interface{}{"country": country, "agents": len(agents)})
	}
	poolSize := len(members)
	if !includeMembers {
		members = nil
	}
	return PoolOverview{Pool: pool, AvailableProxies: available, PoolSize: poolSize, ActiveSessions: active, AverageQuality: avg, Health: health, AgentRedundancy: len(agents), CountryRedundancy: available, Members: members}, nil
}

func (s *Service) Dashboard() (map[string]interface{}, error) {
	pools, err := s.ListPools()
	if err != nil {
		return nil, err
	}
	var events []domain.RoutingEvent
	s.db.Order("created_at desc").Limit(100).Find(&events)
	var rotations, failovers int64
	s.db.Model(&domain.RoutingEvent{}).Where("action = ?", "ProxyRotated").Count(&rotations)
	s.db.Model(&domain.RoutingEvent{}).Where("action = ?", "ProxyFailedOver").Count(&failovers)
	ha := s.HAScore(pools)
	return map[string]interface{}{
		"pools":          pools,
		"events":         events,
		"rotation_count": rotations,
		"failover_count": failovers,
		"ha_score":       ha,
	}, nil
}

func (s *Service) HAScore(pools []PoolOverview) int {
	if len(pools) == 0 {
		return 0
	}
	total := 0
	for _, p := range pools {
		score := p.AverageQuality
		if p.AgentRedundancy >= 2 {
			score += 10
		}
		if p.AvailableProxies >= 2 {
			score += 10
		}
		if p.Health == "dead" {
			score = 0
		}
		if score > 100 {
			score = 100
		}
		total += score
	}
	return total / len(pools)
}

func (s *Service) candidates(input domain.RoutingSelectionInput) ([]*Candidate, error) {
	var proxies []domain.Proxy
	q := s.db.Where("status <> ?", "error")
	if input.ProxyType != "" {
		q = q.Where("type = ?", input.ProxyType)
	}
	if input.Country != "" {
		q = q.Where("country = ?", input.Country)
	}
	region := input.Region
	if region == "" {
		region = input.TargetRegion
	}
	if region != "" {
		q = q.Where("region = ?", region)
	}
	if err := q.Find(&proxies).Error; err != nil {
		return nil, err
	}
	out := make([]*Candidate, 0, len(proxies))
	reserved := s.activeReservations(input.CustomerID)
	for _, p := range proxies {
		c, err := s.candidateForProxy(p.ID, input)
		if err != nil {
			continue
		}
		if !reservationAllows(reserved, p, c.Pool.ID) {
			continue
		}
		if input.Provider != "" && !strings.EqualFold(c.VPNNode.Provider, input.Provider) {
			continue
		}
		if input.ASN != "" && !strings.EqualFold(c.VPNNode.ASN, input.ASN) {
			continue
		}
		if input.ISP != "" && !strings.Contains(strings.ToLower(c.VPNNode.ISP), strings.ToLower(input.ISP)) {
			continue
		}
		if c.Quality.Grade == "Dead" || c.Agent.Status == "offline" {
			continue
		}
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weighted == out[j].Weighted {
			return out[i].Quality.LatencyMs < out[j].Quality.LatencyMs
		}
		return out[i].Weighted > out[j].Weighted
	})
	return out, nil
}

func (s *Service) candidateForProxy(proxyID string, input domain.RoutingSelectionInput) (*Candidate, error) {
	var p domain.Proxy
	if err := s.db.First(&p, "id = ?", proxyID).Error; err != nil {
		return nil, err
	}
	var pool domain.ProxyPool
	country := p.Country
	if country == "" {
		country = input.Country
	}
	if country == "" {
		country = "Unassigned"
	}
	if err := s.db.First(&pool, "country = ?", country).Error; err != nil {
		_ = s.CreatePool(country, p.Region, "weighted_score")
		_ = s.db.First(&pool, "country = ?", country).Error
	}
	quality := s.CalculateQuality(proxyID)
	load := s.activeLoad(proxyID)
	var agent domain.Agent
	_ = s.db.First(&agent, "id = ?", p.AgentID).Error
	var node domain.VPNNode
	_ = s.db.First(&node, "id = ?", p.VPNNodeID).Error
	return &Candidate{Proxy: p, Pool: pool, Quality: quality, Agent: agent, VPNNode: node, ActiveLoad: load, Weighted: quality.Score - load*3}, nil
}

func (s *Service) balance(input domain.RoutingSelectionInput, candidates []*Candidate) *Candidate {
	switch strategy(input.Strategy) {
	case "lowest_latency":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Quality.LatencyMs < candidates[j].Quality.LatencyMs })
	case "least_loaded":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].ActiveLoad < candidates[j].ActiveLoad })
	case "round_robin":
		key := input.Country
		if key == "" {
			key = "global"
		}
		idx := s.rr[key] % len(candidates)
		s.rr[key]++
		return candidates[idx]
	case "health_first", "highest_quality", "weighted_score":
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].Weighted > candidates[j].Weighted })
	}
	return candidates[0]
}

func (s *Service) findSticky(input domain.RoutingSelectionInput) *domain.StickySession {
	if input.CustomerID == "" || input.SessionKey == "" || !isSticky(input.RotationMode) {
		return nil
	}
	var sticky domain.StickySession
	if err := s.db.Where("customer_id = ? AND session_key = ? AND country = ? AND expires_at > ?", input.CustomerID, input.SessionKey, input.Country, time.Now()).Order("expires_at desc").First(&sticky).Error; err != nil {
		return nil
	}
	return &sticky
}

func (s *Service) saveSticky(input domain.RoutingSelectionInput, proxyID string) {
	now := time.Now()
	expires := now.Add(stickyDuration(input.RotationMode))
	sticky := domain.StickySession{ID: uuid.New().String(), CustomerID: input.CustomerID, SessionKey: input.SessionKey, Country: input.Country, ProxyID: proxyID, RotationMode: input.RotationMode, ExpiresAt: expires, CreatedAt: now, UpdatedAt: now}
	_ = s.db.Where("customer_id = ? AND session_key = ? AND country = ?", input.CustomerID, input.SessionKey, input.Country).Delete(&domain.StickySession{}).Error
	_ = s.db.Create(&sticky).Error
}

func (s *Service) activeLoad(proxyID string) int {
	var count int64
	s.db.Model(&domain.CustomerProxyAllocation{}).Where("proxy_id = ? AND status = ?", proxyID, "active").Count(&count)
	return int(count)
}

func (s *Service) activeReservations(customerID string) []domain.ProxyReservation {
	var rows []domain.ProxyReservation
	now := time.Now()
	q := s.db.Where("status = ? AND starts_at <= ? AND (permanent = ? OR expires_at IS NULL OR expires_at > ?)", "active", now, true, now)
	if customerID != "" {
		q = q.Where("customer_id = ?", customerID)
	}
	q.Find(&rows)
	return rows
}

func (s *Service) recordEvent(customerID, proxyID, poolID, action, message string, metadata map[string]interface{}) {
	raw, _ := json.Marshal(metadata)
	_ = s.db.Create(&domain.RoutingEvent{ID: uuid.New().String(), CustomerID: customerID, ProxyID: proxyID, PoolID: poolID, Action: action, Message: message, Metadata: string(raw), CreatedAt: time.Now()}).Error
	_ = s.db.Create(&domain.AuditEvent{ID: uuid.New().String(), ActorID: customerID, ActorType: "routing", Action: action, TargetID: proxyID, TargetType: "proxy", Metadata: string(raw), CreatedAt: time.Now()}).Error
}

func strategy(v string) string {
	if v == "" {
		return "weighted_score"
	}
	return v
}

func isSticky(mode string) bool {
	return mode == "sticky_30m" || mode == "sticky_6h" || mode == "sticky_24h"
}

func stickyDuration(mode string) time.Duration {
	switch mode {
	case "sticky_6h":
		return 6 * time.Hour
	case "sticky_24h":
		return 24 * time.Hour
	default:
		return 30 * time.Minute
	}
}

func countryFromVPNNode(db *gorm.DB, vpnNodeID string) string {
	var node domain.VPNNode
	if vpnNodeID != "" && db.First(&node, "id = ?", vpnNodeID).Error == nil {
		return node.Country
	}
	return ""
}

func memberStatus(p domain.Proxy, q domain.ProxyQualitySnapshot) string {
	if p.Status == "error" || q.Grade == "Dead" {
		return "degraded"
	}
	return "active"
}

func agentOffline(db *gorm.DB, agentID string) bool {
	if agentID == "" {
		return false
	}
	var agent domain.Agent
	return db.First(&agent, "id = ?", agentID).Error == nil && agent.Status == "offline"
}

func reservationAllows(rows []domain.ProxyReservation, p domain.Proxy, poolID string) bool {
	if len(rows) == 0 {
		return true
	}
	for _, r := range rows {
		switch r.Type {
		case "proxy":
			if r.ProxyID == p.ID {
				return true
			}
		case "pool":
			if r.PoolID == poolID || strings.EqualFold(r.Country, p.Country) {
				return true
			}
		case "agent":
			if r.AgentID == p.AgentID {
				return true
			}
		}
	}
	return false
}
