package billing

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
)

type CheckoutRequest struct {
	CustomerID     string  `json:"customer_id"`
	SubscriptionID string  `json:"subscription_id"`
	InvoiceID      string  `json:"invoice_id"`
	Amount         float64 `json:"amount"`
	Currency       string  `json:"currency"`
}

type CheckoutSession struct {
	Provider   string `json:"provider"`
	PaymentRef string `json:"payment_ref"`
	URL        string `json:"url"`
	Status     string `json:"status"`
}

type PaymentProvider interface {
	CreateCheckout(req CheckoutRequest) (*CheckoutSession, error)
	VerifyPayment(paymentRef string) (bool, error)
	RefundPayment(paymentRef string, amount float64) error
	GetPaymentStatus(paymentRef string) (string, error)
}

type MockPaymentProvider struct{}

func (MockPaymentProvider) CreateCheckout(req CheckoutRequest) (*CheckoutSession, error) {
	ref := "mock_" + uuid.New().String()
	return &CheckoutSession{Provider: "mock", PaymentRef: ref, URL: "/mock-checkout/" + ref, Status: "pending"}, nil
}

func (MockPaymentProvider) VerifyPayment(paymentRef string) (bool, error) {
	return strings.HasPrefix(paymentRef, "mock_"), nil
}

func (MockPaymentProvider) RefundPayment(paymentRef string, amount float64) error {
	if !strings.HasPrefix(paymentRef, "mock_") {
		return errors.New("unknown mock payment reference")
	}
	return nil
}

func (MockPaymentProvider) GetPaymentStatus(paymentRef string) (string, error) {
	if !strings.HasPrefix(paymentRef, "mock_") {
		return "unknown", nil
	}
	return "paid", nil
}

type Service struct {
	db       *gorm.DB
	provider PaymentProvider
	dbPath   string
}

func NewService(db *gorm.DB, dbPath string, provider PaymentProvider) *Service {
	if provider == nil {
		provider = MockPaymentProvider{}
	}
	return &Service{db: db, dbPath: dbPath, provider: provider}
}

func (s *Service) EnsureDefaultPlans() {
	defaults := []domain.Plan{
		{ID: "starter-v4", Name: "Starter", Description: "Entry production proxy plan", Price: 19, Currency: "USD", MaxProxies: 5, BandwidthLimitGB: 50, ConcurrentConnections: 25, AllowedCountries: "[]", Status: "active"},
		{ID: "professional-v4", Name: "Professional", Description: "Production plan for automation teams", Price: 79, Currency: "USD", MaxProxies: 25, BandwidthLimitGB: 250, ConcurrentConnections: 100, AllowedCountries: "[]", Status: "active"},
		{ID: "business-v4", Name: "Business", Description: "Higher quota production plan", Price: 249, Currency: "USD", MaxProxies: 100, BandwidthLimitGB: 1000, ConcurrentConnections: 500, AllowedCountries: "[]", Status: "active"},
		{ID: "enterprise-v4", Name: "Enterprise", Description: "Custom operations plan", Price: 999, Currency: "USD", MaxProxies: 500, BandwidthLimitGB: 5000, ConcurrentConnections: 2500, AllowedCountries: "[]", Status: "active"},
	}
	for _, plan := range defaults {
		var existing domain.Plan
		if err := s.db.First(&existing, "id = ?", plan.ID).Error; err == nil {
			continue
		}
		now := time.Now()
		plan.CreatedAt = now
		plan.UpdatedAt = now
		_ = s.db.Create(&plan).Error
	}
}

func (s *Service) CreatePlan(plan *domain.Plan, features map[string]string) error {
	if plan.ID == "" {
		plan.ID = uuid.New().String()
	}
	if plan.Currency == "" {
		plan.Currency = "USD"
	}
	if plan.Status == "" {
		plan.Status = "active"
	}
	now := time.Now()
	plan.CreatedAt = now
	plan.UpdatedAt = now
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(plan).Error; err != nil {
			return err
		}
		for k, v := range features {
			if err := tx.Create(&domain.PlanFeature{ID: uuid.New().String(), PlanID: plan.ID, Key: k, Value: v, CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) UpdatePlan(id string, updates map[string]interface{}) (*domain.Plan, error) {
	updates["updated_at"] = time.Now()
	if err := s.db.Model(&domain.Plan{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	var plan domain.Plan
	if err := s.db.First(&plan, "id = ?", id).Error; err != nil {
		return nil, err
	}
	s.Audit("admin", "system", "PLAN_UPDATED", id, "plan", updates)
	return &plan, nil
}

func (s *Service) ListPlans() ([]domain.Plan, error) {
	var plans []domain.Plan
	err := s.db.Order("price asc").Find(&plans).Error
	return plans, err
}

func (s *Service) CreateSubscription(customerID, planID string, days int, autoRenew bool, status string) (*domain.Subscription, error) {
	if days <= 0 {
		days = 30
	}
	if status == "" {
		status = "pending"
	}
	now := time.Now()
	sub := &domain.Subscription{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		PlanID:     planID,
		Status:     status,
		StartsAt:   now,
		ExpiresAt:  now.Add(time.Duration(days) * 24 * time.Hour),
		AutoRenew:  autoRenew,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.db.Create(sub).Error; err != nil {
		return nil, err
	}
	s.Audit("admin", "system", "SUBSCRIPTION_CREATED", sub.ID, "subscription", map[string]interface{}{"customer_id": customerID, "plan_id": planID})
	return sub, nil
}

func (s *Service) UpdateSubscriptionStatus(id, status string) error {
	if !validSubscriptionStatus(status) {
		return errors.New("invalid subscription status")
	}
	now := time.Now()
	if err := s.db.Model(&domain.Subscription{}).Where("id = ?", id).Updates(map[string]interface{}{"status": status, "updated_at": now}).Error; err != nil {
		return err
	}
	if status == "suspended" || status == "expired" || status == "cancelled" {
		var sub domain.Subscription
		if s.db.First(&sub, "id = ?", id).Error == nil {
			_ = s.SuspendExistingAllocations(sub.CustomerID)
		}
	}
	s.Audit("admin", "system", "SUBSCRIPTION_STATUS_CHANGED", id, "subscription", map[string]interface{}{"status": status})
	return nil
}

func (s *Service) GenerateInvoice(customerID, subscriptionID string) (*domain.Invoice, error) {
	var sub domain.Subscription
	if err := s.db.First(&sub, "id = ? AND customer_id = ?", subscriptionID, customerID).Error; err != nil {
		return nil, err
	}
	var plan domain.Plan
	if err := s.db.First(&plan, "id = ?", sub.PlanID).Error; err != nil {
		return nil, err
	}
	now := time.Now()
	invoice := &domain.Invoice{
		ID:             uuid.New().String(),
		CustomerID:     customerID,
		PlanID:         plan.ID,
		SubscriptionID: sub.ID,
		Amount:         plan.Price,
		Currency:       plan.Currency,
		Status:         "pending",
		CreatedAt:      now,
	}
	item := &domain.InvoiceItem{
		ID:          uuid.New().String(),
		InvoiceID:   invoice.ID,
		Description: fmt.Sprintf("%s subscription", plan.Name),
		Quantity:    1,
		UnitAmount:  plan.Price,
		Amount:      plan.Price,
		CreatedAt:   now,
	}
	session, err := s.provider.CreateCheckout(CheckoutRequest{CustomerID: customerID, SubscriptionID: sub.ID, InvoiceID: invoice.ID, Amount: invoice.Amount, Currency: invoice.Currency})
	if err != nil {
		return nil, err
	}
	invoice.Provider = session.Provider
	invoice.PaymentRef = session.PaymentRef
	invoice.CheckoutURL = session.URL
	return invoice, s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(invoice).Error; err != nil {
			return err
		}
		return tx.Create(item).Error
	})
}

func (s *Service) MarkInvoicePaid(invoiceID string) error {
	now := time.Now()
	var invoice domain.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.Invoice{}).Where("id = ?", invoiceID).Updates(map[string]interface{}{"status": "paid", "paid_at": now}).Error; err != nil {
			return err
		}
		if invoice.SubscriptionID != "" {
			if err := tx.Model(&domain.Subscription{}).Where("id = ?", invoice.SubscriptionID).Updates(map[string]interface{}{"status": "active", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Service) VerifyInvoicePayment(invoiceID string) (*domain.Invoice, error) {
	var invoice domain.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return nil, err
	}
	ok, err := s.provider.VerifyPayment(invoice.PaymentRef)
	if err != nil {
		return nil, err
	}
	if ok && invoice.Status != "paid" {
		if err := s.MarkInvoicePaid(invoice.ID); err != nil {
			return nil, err
		}
		_ = s.db.First(&invoice, "id = ?", invoiceID).Error
	}
	s.Audit("admin", "system", "INVOICE_PAYMENT_VERIFIED", invoiceID, "invoice", map[string]interface{}{"paid": ok})
	return &invoice, nil
}

func (s *Service) RefundInvoice(invoiceID string) (*domain.Invoice, error) {
	var invoice domain.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return nil, err
	}
	if invoice.Status != "paid" {
		return nil, errors.New("only paid invoices can be refunded")
	}
	if err := s.provider.RefundPayment(invoice.PaymentRef, invoice.Amount); err != nil {
		return nil, err
	}
	if err := s.db.Model(&domain.Invoice{}).Where("id = ?", invoiceID).Update("status", "refunded").Error; err != nil {
		return nil, err
	}
	_ = s.db.First(&invoice, "id = ?", invoiceID).Error
	s.Audit("admin", "system", "INVOICE_REFUNDED", invoiceID, "invoice", map[string]interface{}{"amount": invoice.Amount})
	return &invoice, nil
}

func (s *Service) InvoicePaymentStatus(invoiceID string) (map[string]interface{}, error) {
	var invoice domain.Invoice
	if err := s.db.First(&invoice, "id = ?", invoiceID).Error; err != nil {
		return nil, err
	}
	providerStatus, err := s.provider.GetPaymentStatus(invoice.PaymentRef)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"invoice_id":      invoice.ID,
		"invoice_status":  invoice.Status,
		"provider":        invoice.Provider,
		"payment_ref":     invoice.PaymentRef,
		"provider_status": providerStatus,
	}, nil
}

func (s *Service) BillingOverview() (map[string]interface{}, error) {
	var totalCustomers, activeCustomers, activeSubs, pendingInvoices, expiredAccounts int64
	var revenue struct{ Total float64 }
	s.db.Model(&domain.Customer{}).Count(&totalCustomers)
	s.db.Model(&domain.Customer{}).Where("status = ?", "active").Count(&activeCustomers)
	s.db.Model(&domain.Subscription{}).Where("status = ? AND expires_at > ?", "active", time.Now()).Count(&activeSubs)
	s.db.Model(&domain.Invoice{}).Where("status = ?", "pending").Count(&pendingInvoices)
	s.db.Model(&domain.Subscription{}).Where("status = ? OR expires_at <= ?", "expired", time.Now()).Count(&expiredAccounts)
	s.db.Model(&domain.Invoice{}).Select("coalesce(sum(amount), 0) as total").Where("status = ? AND paid_at >= ?", "paid", firstOfMonth()).Scan(&revenue)
	return map[string]interface{}{
		"total_customers":      totalCustomers,
		"active_customers":     activeCustomers,
		"active_subscriptions": activeSubs,
		"monthly_revenue":      revenue.Total,
		"pending_payments":     pendingInvoices,
		"expired_accounts":     expiredAccounts,
	}, nil
}

func (s *Service) ListSubscriptions(limit int) ([]domain.Subscription, error) {
	var subs []domain.Subscription
	err := s.db.Order("created_at desc").Limit(limit).Find(&subs).Error
	return subs, err
}

func (s *Service) ListInvoices(limit int) ([]domain.Invoice, error) {
	var invoices []domain.Invoice
	err := s.db.Order("created_at desc").Limit(limit).Find(&invoices).Error
	return invoices, err
}

func (s *Service) ListAuditEvents(limit int) ([]domain.AuditEvent, error) {
	var events []domain.AuditEvent
	err := s.db.Order("created_at desc").Limit(limit).Find(&events).Error
	return events, err
}

func (s *Service) SuspendCustomer(customerID string) error {
	now := time.Now()
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&domain.Customer{}).Where("id = ?", customerID).Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&domain.Subscription{}).Where("customer_id = ?", customerID).Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND status = ?", customerID, "active").Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error
	})
}

func (s *Service) ActivateCustomer(customerID string) error {
	now := time.Now()
	return s.db.Model(&domain.Customer{}).Where("id = ?", customerID).Updates(map[string]interface{}{"status": "active", "updated_at": now}).Error
}

func (s *Service) UsageDashboard(customerID string) (map[string]interface{}, error) {
	sub, plan, err := s.ActiveCommercialSubscription(customerID)
	if err != nil {
		return map[string]interface{}{"subscription_status": "inactive"}, nil
	}
	var allocs []domain.CustomerProxyAllocation
	if err := s.db.Where("customer_id = ? AND status <> ?", customerID, "deleted").Find(&allocs).Error; err != nil {
		return nil, err
	}
	var in, out uint64
	countries := map[string]bool{}
	ips := map[string]bool{}
	for _, a := range allocs {
		in += a.BandwidthIn
		out += a.BandwidthOut
		if a.Country != "" {
			countries[a.Country] = true
		}
		var p domain.Proxy
		if s.db.First(&p, "id = ?", a.ProxyID).Error == nil {
			ip := p.PublicIP
			if ip == "" {
				ip = p.Host
			}
			if ip != "" {
				ips[ip] = true
			}
		}
	}
	usedGB := float64(in+out) / 1024 / 1024 / 1024
	remaining := float64(plan.BandwidthLimitGB) - usedGB
	if remaining < 0 {
		remaining = 0
	}
	return map[string]interface{}{
		"bandwidth_used_gb":      usedGB,
		"bandwidth_remaining_gb": remaining,
		"active_proxies":         len(allocs),
		"subscription_status":    sub.Status,
		"plan_name":              plan.Name,
		"renewal_date":           sub.ExpiresAt,
		"current_public_ips":     keys(ips),
		"countries_used":         keys(countries),
	}, nil
}

func (s *Service) EnforceAllocation(customerID, country string) (*domain.Subscription, *domain.Plan, error) {
	sub, plan, err := s.ActiveCommercialSubscription(customerID)
	if err != nil {
		return nil, nil, err
	}
	var count int64
	s.db.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND status = ?", customerID, "active").Count(&count)
	if plan.MaxProxies > 0 && int(count) >= plan.MaxProxies {
		return nil, nil, errors.New("plan proxy limit reached")
	}
	if country != "" && !jsonListAllows(plan.AllowedCountries, country) {
		return nil, nil, errors.New("country is not allowed by plan")
	}
	var usage struct {
		BandwidthInTotal  uint64
		BandwidthOutTotal uint64
	}
	s.db.Model(&domain.CustomerProxyAllocation{}).
		Select("coalesce(sum(bandwidth_in),0) as bandwidth_in_total, coalesce(sum(bandwidth_out),0) as bandwidth_out_total").
		Where("customer_id = ?", customerID).
		Scan(&usage)
	if plan.BandwidthLimitGB > 0 && usage.BandwidthInTotal+usage.BandwidthOutTotal >= uint64(plan.BandwidthLimitGB)*1024*1024*1024 {
		return nil, nil, errors.New("bandwidth limit reached")
	}
	return sub, plan, nil
}

func (s *Service) ActiveCommercialSubscription(customerID string) (*domain.Subscription, *domain.Plan, error) {
	var sub domain.Subscription
	if err := s.db.Where("customer_id = ? AND status = ? AND starts_at <= ? AND expires_at > ?", customerID, "active", time.Now(), time.Now()).Order("expires_at desc").First(&sub).Error; err != nil {
		return nil, nil, errors.New("active subscription required")
	}
	var plan domain.Plan
	if err := s.db.First(&plan, "id = ? AND status = ?", sub.PlanID, "active").Error; err != nil {
		return nil, nil, errors.New("active plan required")
	}
	return &sub, &plan, nil
}

func (s *Service) SuspendExistingAllocations(customerID string) error {
	now := time.Now()
	return s.db.Model(&domain.CustomerProxyAllocation{}).Where("customer_id = ? AND status = ?", customerID, "active").Updates(map[string]interface{}{"status": "suspended", "updated_at": now}).Error
}

func (s *Service) Audit(actorID, actorType, action, targetID, targetType string, metadata map[string]interface{}) {
	raw, _ := json.Marshal(metadata)
	_ = s.db.Create(&domain.AuditEvent{ID: uuid.New().String(), ActorID: actorID, ActorType: actorType, Action: action, TargetID: targetID, TargetType: targetType, Metadata: string(raw), CreatedAt: time.Now()}).Error
}

func (s *Service) ExportBackup(format string, configPaths []string) (string, []byte, error) {
	payload, err := s.backupPayload(configPaths)
	if err != nil {
		return "", nil, err
	}
	raw, _ := json.MarshalIndent(payload, "", "  ")
	if format != "zip" {
		return "application/json", raw, nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("backup.json")
	_, _ = w.Write(raw)
	if s.dbPath != "" {
		if dbBytes, err := os.ReadFile(s.dbPath); err == nil {
			w, _ := zw.Create(filepath.Base(s.dbPath))
			_, _ = w.Write(dbBytes)
		}
	}
	_ = zw.Close()
	return "application/zip", buf.Bytes(), nil
}

func (s *Service) backupPayload(configPaths []string) (map[string]interface{}, error) {
	var audits []domain.AuditEvent
	var legacyAudits []domain.AuditLog
	var plans []domain.Plan
	var subs []domain.Subscription
	var invoices []domain.Invoice
	s.db.Order("created_at desc").Limit(5000).Find(&audits)
	s.db.Order("timestamp desc").Limit(5000).Find(&legacyAudits)
	s.db.Find(&plans)
	s.db.Find(&subs)
	s.db.Find(&invoices)
	configs := map[string]string{}
	for _, p := range configPaths {
		if b, err := os.ReadFile(p); err == nil {
			configs[p] = string(b)
		}
	}
	return map[string]interface{}{
		"exported_at":   time.Now().Format(time.RFC3339),
		"plans":         plans,
		"subscriptions": subs,
		"invoices":      invoices,
		"audit_events":  audits,
		"audit_logs":    legacyAudits,
		"configs":       configs,
	}, nil
}

func validSubscriptionStatus(status string) bool {
	switch status {
	case "active", "expired", "cancelled", "suspended", "pending":
		return true
	default:
		return false
	}
}

func firstOfMonth() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
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

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
