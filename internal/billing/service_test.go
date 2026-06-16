package billing

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/repository"
)

func TestPhase4ABillingLifecycleAndEnforcement(t *testing.T) {
	db, err := repository.NewSQLiteDB(t.TempDir() + "/billing.db")
	if err != nil {
		t.Fatalf("db init: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	defer sqlDB.Close()
	svc := NewService(db, t.TempDir()+"/billing.db", MockPaymentProvider{})

	customerID := uuid.New().String()
	if err := db.Create(&domain.Customer{
		ID:           customerID,
		Email:        "phase4a@example.com",
		PasswordHash: "hash",
		Status:       "active",
		Role:         "customer",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}).Error; err != nil {
		t.Fatalf("customer create: %v", err)
	}

	plan := &domain.Plan{
		Name:                  "Phase4A Test",
		Description:           "test plan",
		Price:                 10,
		Currency:              "USD",
		MaxProxies:            1,
		BandwidthLimitGB:      1,
		ConcurrentConnections: 2,
		AllowedCountries:      `["Vietnam"]`,
		Status:                "active",
	}
	if err := svc.CreatePlan(plan, map[string]string{"rotation": "static"}); err != nil {
		t.Fatalf("plan create: %v", err)
	}

	sub, err := svc.CreateSubscription(customerID, plan.ID, 30, true, "pending")
	if err != nil {
		t.Fatalf("subscription create: %v", err)
	}
	invoice, err := svc.GenerateInvoice(customerID, sub.ID)
	if err != nil {
		t.Fatalf("invoice generate: %v", err)
	}
	if invoice.Status != "pending" || invoice.PaymentRef == "" {
		t.Fatalf("unexpected invoice: %+v", invoice)
	}
	if err := svc.MarkInvoicePaid(invoice.ID); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	status, err := svc.InvoicePaymentStatus(invoice.ID)
	if err != nil {
		t.Fatalf("payment status: %v", err)
	}
	if status["provider_status"] != "paid" {
		t.Fatalf("unexpected provider status: %+v", status)
	}
	if _, err := svc.VerifyInvoicePayment(invoice.ID); err != nil {
		t.Fatalf("verify payment: %v", err)
	}
	if _, _, err := svc.EnforceAllocation(customerID, "Vietnam"); err != nil {
		t.Fatalf("expected active enforcement success: %v", err)
	}
	if _, _, err := svc.EnforceAllocation(customerID, "Japan"); err == nil {
		t.Fatalf("expected country enforcement failure")
	}

	if err := db.Create(&domain.CustomerProxyAllocation{
		ID:             uuid.New().String(),
		CustomerID:     customerID,
		SubscriptionID: sub.ID,
		ProxyID:        uuid.New().String(),
		CredentialID:   uuid.New().String(),
		RotationMode:   "static",
		Country:        "Vietnam",
		Status:         "active",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}).Error; err != nil {
		t.Fatalf("allocation seed: %v", err)
	}
	if _, _, err := svc.EnforceAllocation(customerID, "Vietnam"); err == nil {
		t.Fatalf("expected max proxy enforcement failure")
	}
	if err := svc.UpdateSubscriptionStatus(sub.ID, "suspended"); err != nil {
		t.Fatalf("suspend subscription: %v", err)
	}
	if _, _, err := svc.EnforceAllocation(customerID, "Vietnam"); err == nil {
		t.Fatalf("expected suspended subscription to block allocation")
	}
	if _, err := svc.RefundInvoice(invoice.ID); err != nil {
		t.Fatalf("refund invoice: %v", err)
	}

	contentType, payload, err := svc.ExportBackup("json", nil)
	if err != nil {
		t.Fatalf("backup export: %v", err)
	}
	if contentType != "application/json" || len(payload) == 0 {
		t.Fatalf("unexpected backup response: %s %d", contentType, len(payload))
	}
}
