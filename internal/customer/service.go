package customer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
)

type Service struct {
	db        *gorm.DB
	proxyRepo domain.ProxyRepository
	auditRepo domain.AuditLogRepository
	enforcer  SubscriptionEnforcer
	router    RoutingSelector
}

type SubscriptionEnforcer interface {
	EnforceAllocation(customerID, country string) (*domain.Subscription, *domain.Plan, error)
	ActiveCommercialSubscription(customerID string) (*domain.Subscription, *domain.Plan, error)
	UsageDashboard(customerID string) (map[string]interface{}, error)
}

type RoutingSelector interface {
	SelectDomainProxy(input domain.RoutingSelectionInput) (*domain.Proxy, error)
}

type Principal struct {
	Customer *domain.Customer
	APIKeyID string
}

type AllocatedProxy struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	Password     string `json:"password,omitempty"`
	Country      string `json:"country"`
	PublicIP     string `json:"public_ip"`
	RotationMode string `json:"rotationMode"`
	Health       string `json:"health"`
	Status       string `json:"status"`
	BandwidthIn  uint64 `json:"bandwidth_in"`
	BandwidthOut uint64 `json:"bandwidth_out"`
}

func NewService(db *gorm.DB, proxyRepo domain.ProxyRepository, auditRepo domain.AuditLogRepository) *Service {
	return &Service{db: db, proxyRepo: proxyRepo, auditRepo: auditRepo}
}

func (s *Service) SetSubscriptionEnforcer(enforcer SubscriptionEnforcer) {
	s.enforcer = enforcer
}

func (s *Service) SetRoutingSelector(router RoutingSelector) {
	s.router = router
}

func (s *Service) EnsureDefaultPlans() {
	defaults := []domain.ProxyPlan{
		{ID: "starter", Name: "Starter", MaxProxies: 5, AllowedCountries: "[]", BandwidthLimitGB: 50, RotationModes: `["static","sticky_30m"]`, ConcurrentConnections: 25, Price: 19, Status: "active"},
		{ID: "professional", Name: "Professional", MaxProxies: 25, AllowedCountries: "[]", BandwidthLimitGB: 250, RotationModes: `["static","sticky_30m","sticky_6h","rotating"]`, ConcurrentConnections: 100, Price: 79, Status: "active"},
		{ID: "business", Name: "Business", MaxProxies: 100, AllowedCountries: "[]", BandwidthLimitGB: 1000, RotationModes: `["static","sticky_30m","sticky_6h","sticky_24h","rotating"]`, ConcurrentConnections: 500, Price: 249, Status: "active"},
	}
	for _, plan := range defaults {
		var existing domain.ProxyPlan
		if err := s.db.First(&existing, "id = ?", plan.ID).Error; err == nil {
			continue
		}
		now := time.Now()
		plan.CreatedAt = now
		plan.UpdatedAt = now
		_ = s.db.Create(&plan).Error
	}
}

func (s *Service) Register(email, password string) (*domain.Customer, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || password == "" {
		return nil, "", errors.New("email and password are required")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	c := &domain.Customer{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: string(hash),
		Status:       "active",
		Role:         "customer",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.db.Create(c).Error; err != nil {
		return nil, "", err
	}
	_ = s.db.Create(&domain.CustomerCredential{ID: uuid.New().String(), CustomerID: c.ID, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now}).Error
	token, err := s.createSession(c.ID)
	if err != nil {
		return nil, "", err
	}
	s.audit("CUSTOMER_REGISTERED", fmt.Sprintf("Customer registered: %s", c.Email))
	return c, token, nil
}

func (s *Service) Login(email, password string) (*domain.Customer, string, error) {
	var c domain.Customer
	if err := s.db.First(&c, "email = ?", strings.ToLower(strings.TrimSpace(email))).Error; err != nil {
		return nil, "", errors.New("invalid credentials")
	}
	if c.Status != "active" {
		return nil, "", errors.New("customer is not active")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(c.PasswordHash), []byte(password)); err != nil {
		return nil, "", errors.New("invalid credentials")
	}
	token, err := s.createSession(c.ID)
	if err != nil {
		return nil, "", err
	}
	return &c, token, nil
}

func (s *Service) AuthenticateBearer(header string) (*Principal, error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return nil, errors.New("missing bearer token")
	}
	token := strings.TrimSpace(header[len(prefix):])
	hash := sha256Hex(token)

	var session domain.CustomerSession
	if err := s.db.First(&session, "token_hash = ? AND revoked_at IS NULL", hash).Error; err == nil {
		if session.ExpiresAt.Before(time.Now()) {
			return nil, errors.New("session expired")
		}
		customer, err := s.getActiveCustomer(session.CustomerID)
		if err != nil {
			return nil, err
		}
		return &Principal{Customer: customer}, nil
	}

	var apiKey domain.CustomerApiKey
	if err := s.db.First(&apiKey, "key_hash = ? AND status = ?", hash, "active").Error; err == nil {
		customer, err := s.getActiveCustomer(apiKey.CustomerID)
		if err != nil {
			return nil, err
		}
		now := time.Now()
		apiKey.LastUsedAt = &now
		_ = s.db.Save(&apiKey).Error
		return &Principal{Customer: customer, APIKeyID: apiKey.ID}, nil
	}

	return nil, errors.New("invalid token")
}

func (s *Service) CreateAPIKey(customerID, name string) (*domain.CustomerApiKey, string, error) {
	plain, err := randomHex(32)
	if err != nil {
		return nil, "", err
	}
	prefix := plain
	if len(prefix) > 10 {
		prefix = prefix[:10]
	}
	key := &domain.CustomerApiKey{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		Name:       name,
		KeyHash:    sha256Hex(plain),
		Prefix:     prefix,
		Status:     "active",
		CreatedAt:  time.Now(),
	}
	if err := s.db.Create(key).Error; err != nil {
		return nil, "", err
	}
	return key, plain, nil
}

func (s *Service) DeleteAPIKey(customerID, id string) error {
	return s.db.Model(&domain.CustomerApiKey{}).Where("id = ? AND customer_id = ?", id, customerID).Update("status", "revoked").Error
}

func (s *Service) ListPlans() ([]domain.ProxyPlan, error) {
	var plans []domain.ProxyPlan
	err := s.db.Where("status = ?", "active").Order("price asc").Find(&plans).Error
	return plans, err
}

func (s *Service) CreatePlan(plan *domain.ProxyPlan) error {
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	now := time.Now()
	plan.CreatedAt = now
	plan.UpdatedAt = now
	if plan.Status == "" {
		plan.Status = "active"
	}
	return s.db.Create(plan).Error
}

func (s *Service) ActivateSubscription(customerID, planID string, days int) (*domain.CustomerSubscription, error) {
	if days == 0 {
		days = 30
	}
	now := time.Now()
	sub := &domain.CustomerSubscription{
		ID:           uuid.New().String(),
		CustomerID:   customerID,
		PlanID:       planID,
		Status:       "active",
		StartsAt:     now,
		ExpiresAt:    now.Add(time.Duration(days) * 24 * time.Hour),
		UsageResetAt: now.Add(30 * 24 * time.Hour),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.db.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *Service) AllocateProxy(customerID, country, rotationMode, proxyType string) (*AllocatedProxy, error) {
	sub, plan, err := s.activeSubscription(customerID)
	if err != nil {
		return nil, err
	}
	if s.enforcer != nil {
		if _, _, err := s.enforcer.EnforceAllocation(customerID, country); err != nil {
			return nil, err
		}
	}
	if err := s.checkAllocationAllowed(customerID, plan, country, rotationMode); err != nil {
		return nil, err
	}

	var chosen *domain.Proxy
	if s.router != nil {
		routed, err := s.router.SelectDomainProxy(domain.RoutingSelectionInput{
			CustomerID:   customerID,
			Country:      country,
			ProxyType:    proxyType,
			PlanID:       plan.ID,
			RotationMode: defaultRotation(rotationMode),
			SessionKey:   customerID + ":" + country,
			Strategy:     "weighted_score",
		})
		if err == nil && routed != nil && routed.ID != "" {
			chosen = routed
		}
	}
	if chosen == nil {
		var candidates []domain.Proxy
		q := s.db.Where("status <> ?", "error")
		if proxyType != "" {
			q = q.Where("type = ?", proxyType)
		}
		if country != "" {
			q = q.Where("country = ? OR agent_id IN (?)", country, s.db.Model(&domain.Agent{}).Select("id").Where("status <> ?", "offline"))
		}
		if err := q.Order("updated_at desc").Find(&candidates).Error; err != nil {
			return nil, err
		}

		allocated := map[string]bool{}
		var existing []domain.CustomerProxyAllocation
		_ = s.db.Where("status = ?", "active").Find(&existing).Error
		for _, a := range existing {
			allocated[a.ProxyID] = true
		}

		for i := range candidates {
			if !allocated[candidates[i].ID] && candidates[i].Status != "error" {
				chosen = &candidates[i]
				break
			}
		}
		if chosen == nil {
			return nil, errors.New("no healthy unallocated proxy available for requested filters")
		}
	}

	username, password, passHash, err := generateProxyCredential()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	cred := &domain.CustomerProxyCredential{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: passHash,
		CustomerID:   customerID,
		ProxyID:      chosen.ID,
		Status:       "active",
		CreatedAt:    now,
	}
	alloc := &domain.CustomerProxyAllocation{
		ID:             uuid.New().String(),
		CustomerID:     customerID,
		SubscriptionID: sub.ID,
		ProxyID:        chosen.ID,
		CredentialID:   cred.ID,
		RotationMode:   defaultRotation(rotationMode),
		Country:        countryFromProxy(*chosen, country),
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(cred).Error; err != nil {
			return err
		}
		if err := tx.Create(alloc).Error; err != nil {
			return err
		}
		return tx.Model(&domain.Proxy{}).Where("id = ?", chosen.ID).Updates(map[string]interface{}{
			"username": username,
			"password": passHash,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	s.audit("CUSTOMER_PROXY_ALLOCATED", fmt.Sprintf("Customer %s allocated proxy %s", customerID, chosen.ID))
	return s.toAllocatedProxy(*alloc, *chosen, *cred, password), nil
}

func (s *Service) ListAllocatedProxies(customerID string) ([]AllocatedProxy, error) {
	var allocs []domain.CustomerProxyAllocation
	if err := s.db.Where("customer_id = ? AND status <> ?", customerID, "deleted").Order("created_at desc").Find(&allocs).Error; err != nil {
		return nil, err
	}
	out := make([]AllocatedProxy, 0, len(allocs))
	for _, alloc := range allocs {
		var p domain.Proxy
		var cred domain.CustomerProxyCredential
		if s.db.First(&p, "id = ?", alloc.ProxyID).Error != nil || s.db.First(&cred, "id = ?", alloc.CredentialID).Error != nil {
			continue
		}
		out = append(out, *s.toAllocatedProxy(alloc, p, cred, ""))
	}
	return out, nil
}

func (s *Service) RotateCredential(customerID, allocationID string) (*AllocatedProxy, error) {
	var alloc domain.CustomerProxyAllocation
	if err := s.db.First(&alloc, "id = ? AND customer_id = ?", allocationID, customerID).Error; err != nil {
		return nil, err
	}
	username, password, passHash, err := generateProxyCredential()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var proxy domain.Proxy
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.CustomerProxyCredential{}).Where("id = ?", alloc.CredentialID).Updates(map[string]interface{}{
			"username": username, "password_hash": passHash, "rotated_at": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.Proxy{}).Where("id = ?", alloc.ProxyID).Updates(map[string]interface{}{
			"username": username, "password": passHash,
		}).Error; err != nil {
			return err
		}
		return tx.First(&proxy, "id = ?", alloc.ProxyID).Error
	})
	if err != nil {
		return nil, err
	}
	cred := domain.CustomerProxyCredential{ID: alloc.CredentialID, Username: username, PasswordHash: passHash, CustomerID: customerID, ProxyID: alloc.ProxyID, Status: "active", RotatedAt: &now}
	return s.toAllocatedProxy(alloc, proxy, cred, password), nil
}

func (s *Service) ReleaseProxy(customerID, allocationID string) error {
	now := time.Now()
	return s.db.Model(&domain.CustomerProxyAllocation{}).
		Where("id = ? AND customer_id = ?", allocationID, customerID).
		Updates(map[string]interface{}{"status": "deleted", "updated_at": now}).Error
}

func (s *Service) RotateProxy(customerID, allocationID string) (*AllocatedProxy, error) {
	if err := s.ReleaseProxy(customerID, allocationID); err != nil {
		return nil, err
	}
	return s.AllocateProxy(customerID, "", "rotating", "")
}

func (s *Service) Usage(customerID string) (map[string]interface{}, error) {
	if s.enforcer != nil {
		if usage, err := s.enforcer.UsageDashboard(customerID); err == nil && usage["subscription_status"] != "inactive" {
			return usage, nil
		}
	}
	var allocs []domain.CustomerProxyAllocation
	if err := s.db.Where("customer_id = ?", customerID).Find(&allocs).Error; err != nil {
		return nil, err
	}
	var in, out uint64
	for _, a := range allocs {
		in += a.BandwidthIn
		out += a.BandwidthOut
	}
	return map[string]interface{}{
		"bandwidth_in":  in,
		"bandwidth_out": out,
		"proxy_count":   len(allocs),
	}, nil
}

func (s *Service) ValidateProxyCredential(proxyID, username, password string) bool {
	var cred domain.CustomerProxyCredential
	if err := s.db.First(&cred, "proxy_id = ? AND username = ? AND status = ?", proxyID, username, "active").Error; err != nil {
		return false
	}
	var alloc domain.CustomerProxyAllocation
	if err := s.db.First(&alloc, "proxy_id = ? AND customer_id = ? AND status = ?", proxyID, cred.CustomerID, "active").Error; err != nil {
		return false
	}
	if _, _, err := s.activeSubscription(cred.CustomerID); err != nil {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)) == nil
}

func (s *Service) ExportProxies(customerID, format string) (string, string, error) {
	items, err := s.ListAllocatedProxies(customerID)
	if err != nil {
		return "", "", err
	}
	switch format {
	case "json":
		b, _ := json.MarshalIndent(items, "", "  ")
		return "application/json", string(b), nil
	case "csv":
		var b strings.Builder
		b.WriteString("host,port,username,password,type,country,rotation_mode\n")
		for _, p := range items {
			b.WriteString(fmt.Sprintf("%s,%d,%s,,%s,%s,%s\n", p.Host, p.Port, p.Username, p.Type, p.Country, p.RotationMode))
		}
		return "text/csv", b.String(), nil
	default:
		lines := make([]string, 0, len(items))
		for _, p := range items {
			lines = append(lines, fmt.Sprintf("%s:%d:%s:", p.Host, p.Port, p.Username))
		}
		return "text/plain", strings.Join(lines, "\n"), nil
	}
}

func (s *Service) activeSubscription(customerID string) (*domain.CustomerSubscription, *domain.ProxyPlan, error) {
	if s.enforcer != nil {
		if sub, plan, err := s.enforcer.ActiveCommercialSubscription(customerID); err == nil {
			return &domain.CustomerSubscription{
					ID:         sub.ID,
					CustomerID: sub.CustomerID,
					PlanID:     sub.PlanID,
					Status:     sub.Status,
					StartsAt:   sub.StartsAt,
					ExpiresAt:  sub.ExpiresAt,
					CreatedAt:  sub.CreatedAt,
					UpdatedAt:  sub.UpdatedAt,
				}, &domain.ProxyPlan{
					ID:                    plan.ID,
					Name:                  plan.Name,
					MaxProxies:            plan.MaxProxies,
					AllowedCountries:      plan.AllowedCountries,
					BandwidthLimitGB:      plan.BandwidthLimitGB,
					ConcurrentConnections: plan.ConcurrentConnections,
					Price:                 plan.Price,
					Status:                plan.Status,
					CreatedAt:             plan.CreatedAt,
					UpdatedAt:             plan.UpdatedAt,
				}, nil
		}
	}
	var sub domain.CustomerSubscription
	if err := s.db.Where("customer_id = ? AND status = ? AND expires_at > ?", customerID, "active", time.Now()).Order("expires_at desc").First(&sub).Error; err != nil {
		return nil, nil, errors.New("active subscription required")
	}
	var plan domain.ProxyPlan
	if err := s.db.First(&plan, "id = ? AND status = ?", sub.PlanID, "active").Error; err != nil {
		return nil, nil, errors.New("active plan required")
	}
	return &sub, &plan, nil
}

func (s *Service) checkAllocationAllowed(customerID string, plan *domain.ProxyPlan, country, rotationMode string) error {
	var count int64
	s.db.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND status = ?", customerID, "active").Count(&count)
	if plan.MaxProxies > 0 && int(count) >= plan.MaxProxies {
		return errors.New("plan proxy limit reached")
	}
	if country != "" && !jsonListAllows(plan.AllowedCountries, country) {
		return errors.New("country is not allowed by plan")
	}
	if rotationMode != "" && !jsonListAllows(plan.RotationModes, rotationMode) {
		return errors.New("rotation mode is not allowed by plan")
	}
	return nil
}

func (s *Service) getActiveCustomer(id string) (*domain.Customer, error) {
	var c domain.Customer
	if err := s.db.First(&c, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if c.Status != "active" {
		return nil, errors.New("customer is not active")
	}
	return &c, nil
}

func (s *Service) createSession(customerID string) (string, error) {
	token, err := randomHex(32)
	if err != nil {
		return "", err
	}
	session := &domain.CustomerSession{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		TokenHash:  sha256Hex(token),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
		CreatedAt:  time.Now(),
	}
	return token, s.db.Create(session).Error
}

func (s *Service) toAllocatedProxy(alloc domain.CustomerProxyAllocation, p domain.Proxy, cred domain.CustomerProxyCredential, plainPassword string) *AllocatedProxy {
	host := p.PublicIP
	if host == "" {
		host = p.Host
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return &AllocatedProxy{
		ID:           alloc.ID,
		Type:         p.Type,
		Host:         host,
		Port:         p.Port,
		Username:     cred.Username,
		Password:     plainPassword,
		Country:      countryFromProxy(p, alloc.Country),
		PublicIP:     p.PublicIP,
		RotationMode: alloc.RotationMode,
		Health:       p.Status,
		Status:       alloc.Status,
		BandwidthIn:  alloc.BandwidthIn,
		BandwidthOut: alloc.BandwidthOut,
	}
}

func (s *Service) audit(action, details string) {
	_ = s.auditRepo.Create(&domain.AuditLog{Action: action, Details: details, Timestamp: time.Now()})
}

func generateProxyCredential() (string, string, string, error) {
	userSuffix, err := randomHex(5)
	if err != nil {
		return "", "", "", err
	}
	pass, err := randomHex(10)
	if err != nil {
		return "", "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return "", "", "", err
	}
	return "user_" + userSuffix, pass, string(hash), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func jsonListAllows(raw, value string) bool {
	if raw == "" || raw == "[]" {
		return true
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return true
	}
	for _, v := range values {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

func defaultRotation(mode string) string {
	if mode == "" {
		return "static"
	}
	return mode
}

func countryFromProxy(p domain.Proxy, fallback string) string {
	if p.Country != "" {
		return p.Country
	}
	return fallback
}
