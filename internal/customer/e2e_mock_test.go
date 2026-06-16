package customer

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"vpn-to-proxy/internal/abuse"
	"vpn-to-proxy/internal/billing"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/repository"
	"vpn-to-proxy/internal/routing"
)

func TestMockCustomerAllocationRoutingSecurityAndUsage(t *testing.T) {
	db, err := repository.NewSQLiteDB(t.TempDir() + "/mock_e2e.db")
	if err != nil {
		t.Fatalf("db init: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	defer sqlDB.Close()

	proxyRepo := repository.NewProxyRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	customerSvc := NewService(db, proxyRepo, auditRepo)
	billingSvc := billing.NewService(db, t.TempDir()+"/mock_e2e.db", billing.MockPaymentProvider{})
	routingSvc := routing.NewService(db)
	abuseSvc := abuse.NewService(db)
	customerSvc.SetSubscriptionEnforcer(billingSvc)
	customerSvc.SetRoutingSelector(routingSvc)

	now := time.Now()
	customerID := uuid.New().String()
	if err := db.Create(&domain.Customer{ID: customerID, Email: "mock-e2e@example.com", PasswordHash: "hash", Status: "active", Role: "customer", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatalf("customer seed: %v", err)
	}
	plan := &domain.Plan{Name: "Mock E2E", Description: "mock", Price: 1, Currency: "USD", MaxProxies: 2, BandwidthLimitGB: 1, ConcurrentConnections: 4, AllowedCountries: `["Vietnam"]`, Status: "active"}
	if err := billingSvc.CreatePlan(plan, nil); err != nil {
		t.Fatalf("plan create: %v", err)
	}
	if _, err := billingSvc.CreateSubscription(customerID, plan.ID, 30, true, "active"); err != nil {
		t.Fatalf("subscription create: %v", err)
	}
	agent := domain.Agent{ID: "mock-agent-vn", Name: "mock-agent-vn", Hostname: "mock", IPAddress: "127.0.0.1", Status: "healthy", LastHeartbeatAt: now, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("agent seed: %v", err)
	}
	node := domain.VPNNode{ID: "mock-vpn-vn", Name: "Mock Vietnam", Provider: "expressvpn_mock", Type: "expressvpn_mock", Status: "connected", Country: "Vietnam", Region: "North", PublicIP: "203.0.113.10", AgentID: agent.ID, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("vpn node seed: %v", err)
	}
	proxy := domain.Proxy{ID: "mock-proxy-vn", VPNNodeID: node.ID, Port: 31080, Type: "socks5", Status: "running", BindIP: "127.0.0.1", Host: "127.0.0.1", PublicIP: node.PublicIP, Country: "Vietnam", Region: "North", AgentID: agent.ID, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&proxy).Error; err != nil {
		t.Fatalf("proxy seed: %v", err)
	}
	if err := db.Create(&domain.HealthMetric{TargetID: proxy.ID, TargetType: "proxy", LatencyMs: 20, Status: "online", CheckedAt: now}).Error; err != nil {
		t.Fatalf("health seed: %v", err)
	}

	allocated, err := customerSvc.AllocateProxy(customerID, "Vietnam", "sticky_30m", "socks5")
	if err != nil {
		t.Fatalf("allocate proxy: %v", err)
	}
	if allocated.Host == "" || allocated.Port != 31080 || allocated.Username == "" || allocated.Password == "" {
		t.Fatalf("unexpected allocation: %+v", allocated)
	}
	auth := abuseSvc.ValidateProxyCredential("mock-proxy-vn", allocated.Username, allocated.Password, "127.0.0.1")
	if !auth.Allowed || auth.CustomerID != customerID {
		t.Fatalf("expected proxy credential auth success, got %+v", auth)
	}
	abuseSvc.RecordUsage(customerID, "mock-proxy-vn", 1024, 2048)
	var usage domain.UsageMetric
	if err := db.First(&usage, "customer_id = ? AND proxy_id = ?", customerID, "mock-proxy-vn").Error; err != nil {
		t.Fatalf("usage metric expected: %v", err)
	}
	if err := customerSvc.ReleaseProxy(customerID, allocated.ID); err != nil {
		t.Fatalf("release proxy: %v", err)
	}
}
