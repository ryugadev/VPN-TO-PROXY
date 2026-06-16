package abuse

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/repository"
)

func TestValidateProxyCredentialHonorsIPWhitelist(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	svc := NewService(db)
	customerID, proxyID := seedCustomerProxyCredential(t, db, "secret")

	if _, err := svc.AddWhitelist(customerID, "127.0.0.1", "", "local only"); err != nil {
		t.Fatalf("add whitelist: %v", err)
	}
	allowed := svc.ValidateProxyCredential(proxyID, "test-user", "secret", "127.0.0.1:5000")
	if !allowed.Allowed || allowed.CustomerID != customerID {
		t.Fatalf("expected whitelisted credential success, got %+v", allowed)
	}
	denied := svc.ValidateProxyCredential(proxyID, "test-user", "secret", "10.0.0.5")
	if denied.Allowed {
		t.Fatalf("expected non-whitelisted client IP to be denied")
	}

	var risk domain.CustomerRiskScore
	if err := db.First(&risk, "customer_id = ?", customerID).Error; err != nil {
		t.Fatalf("expected risk score after whitelist denial: %v", err)
	}
	if risk.Score == 0 {
		t.Fatalf("expected positive risk score")
	}
}

func TestGuardConnectionBlocksTargetsAndRecordsEvent(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	svc := NewService(db)
	customerID, proxyID := seedCustomerProxyCredential(t, db, "secret")

	if _, err := svc.AddBlockedTarget("domain", "blocked.example", "policy"); err != nil {
		t.Fatalf("add blocked target: %v", err)
	}
	release, err := svc.GuardConnection(proxyID, customerID, "127.0.0.1", "api.blocked.example:443")
	if err == nil {
		release()
		t.Fatalf("expected blocked target error")
	}

	var event domain.AbuseEvent
	if err := db.First(&event, "customer_id = ? AND rule_id = ?", customerID, "blocked-target").Error; err != nil {
		t.Fatalf("expected blocked target event: %v", err)
	}
}

func TestFailedAuthRaisesRiskAndSuspendsCredential(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	svc := NewService(db)
	customerID, proxyID := seedCustomerProxyCredential(t, db, "secret")

	for i := 0; i < 50; i++ {
		svc.RecordFailedAuth(customerID, proxyID, "test-user", "127.0.0.1")
	}

	var cred domain.CustomerProxyCredential
	if err := db.First(&cred, "proxy_id = ? AND username = ?", proxyID, "test-user").Error; err != nil {
		t.Fatalf("credential lookup: %v", err)
	}
	if cred.Status != "suspended" {
		t.Fatalf("expected credential suspended, got %q", cred.Status)
	}
	var eventCount int64
	db.Model(&domain.AbuseEvent{}).Where("customer_id = ? AND rule_id = ?", customerID, "failed-auth-suspend").Count(&eventCount)
	if eventCount == 0 {
		t.Fatalf("expected failed auth suspend event")
	}
}

func TestAPIRateLimit(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	svc := NewService(db)

	if !svc.CheckAPIRate("customer-1", "api", 2, time.Minute) {
		t.Fatalf("first request should pass")
	}
	if !svc.CheckAPIRate("customer-1", "api", 2, time.Minute) {
		t.Fatalf("second request should pass")
	}
	if svc.CheckAPIRate("customer-1", "api", 2, time.Minute) {
		t.Fatalf("third request should be rate limited")
	}
}

func testDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db, err := repository.NewSQLiteDB(t.TempDir() + "/abuse.db")
	if err != nil {
		t.Fatalf("db init: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	return db, func() { _ = sqlDB.Close() }
}

func seedCustomerProxyCredential(t *testing.T, db *gorm.DB, password string) (string, string) {
	t.Helper()
	now := time.Now()
	customerID := uuid.New().String()
	proxyID := uuid.New().String()
	credentialID := uuid.New().String()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := db.Create(&domain.Customer{
		ID:           customerID,
		Email:        customerID + "@example.com",
		PasswordHash: "hash",
		Status:       "active",
		Role:         "customer",
		CreatedAt:    now,
		UpdatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create customer: %v", err)
	}
	if err := db.Create(&domain.Proxy{
		ID:        proxyID,
		Port:      20000,
		Type:      "socks5",
		Status:    "running",
		BindIP:    "127.0.0.1",
		Host:      "127.0.0.1",
		CreatedAt: now,
		UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if err := db.Create(&domain.CustomerProxyCredential{
		ID:           credentialID,
		Username:     "test-user",
		PasswordHash: string(hash),
		CustomerID:   customerID,
		ProxyID:      proxyID,
		Status:       "active",
		CreatedAt:    now,
	}).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	return customerID, proxyID
}
