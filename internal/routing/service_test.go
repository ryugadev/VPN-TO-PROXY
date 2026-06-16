package routing

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/repository"
)

func TestPoolQualityAndSmartSelection(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	f := seedRoutingFixture(t, db)
	svc := NewService(db)

	if err := svc.SyncPools(); err != nil {
		t.Fatalf("sync pools: %v", err)
	}
	overview, err := svc.PoolOverview("Vietnam", true)
	if err != nil {
		t.Fatalf("pool overview: %v", err)
	}
	if overview.PoolSize != 2 || overview.AvailableProxies == 0 {
		t.Fatalf("unexpected pool overview: %+v", overview)
	}
	quality := svc.CalculateQuality(f.FastProxyID)
	if quality.Score <= 0 || quality.Grade == "Dead" {
		t.Fatalf("expected usable quality score: %+v", quality)
	}

	selected, err := svc.SelectProxy(domain.RoutingSelectionInput{Country: "Vietnam", Strategy: "weighted_score"})
	if err != nil {
		t.Fatalf("select proxy: %v", err)
	}
	if selected.Proxy.ID != f.FastProxyID {
		t.Fatalf("expected fast proxy selected, got %s", selected.Proxy.ID)
	}
}

func TestStickySessionReturnsSameProxy(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	seedRoutingFixture(t, db)
	svc := NewService(db)

	input := domain.RoutingSelectionInput{CustomerID: "customer-1", Country: "Vietnam", SessionKey: "browser-1", RotationMode: "sticky_30m"}
	first, err := svc.SelectProxy(input)
	if err != nil {
		t.Fatalf("first select: %v", err)
	}
	second, err := svc.SelectProxy(input)
	if err != nil {
		t.Fatalf("second select: %v", err)
	}
	if first.Proxy.ID != second.Proxy.ID {
		t.Fatalf("expected sticky proxy %s, got %s", first.Proxy.ID, second.Proxy.ID)
	}
}

func TestRotationFailoverFiltersAndReservation(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	f := seedRoutingFixture(t, db)
	svc := NewService(db)

	byASN, err := svc.SelectProxy(domain.RoutingSelectionInput{Country: "Vietnam", ASN: "7552"})
	if err != nil {
		t.Fatalf("asn select: %v", err)
	}
	if byASN.VPNNode.ASN != "7552" {
		t.Fatalf("expected ASN 7552, got %s", byASN.VPNNode.ASN)
	}
	byGeo, err := svc.SelectProxy(domain.RoutingSelectionInput{Country: "Vietnam", TargetRegion: "South"})
	if err != nil {
		t.Fatalf("geo select: %v", err)
	}
	if byGeo.Proxy.Region != "South" {
		t.Fatalf("expected South region, got %s", byGeo.Proxy.Region)
	}

	if err := svc.CreateReservation(&domain.ProxyReservation{CustomerID: "enterprise-1", ProxyID: f.SlowProxyID, Country: "Vietnam", Type: "proxy", Permanent: true}); err != nil {
		t.Fatalf("reservation: %v", err)
	}
	reserved, err := svc.SelectProxy(domain.RoutingSelectionInput{CustomerID: "enterprise-1", Country: "Vietnam"})
	if err != nil {
		t.Fatalf("reserved select: %v", err)
	}
	if reserved.Proxy.ID != f.SlowProxyID {
		t.Fatalf("expected reserved proxy %s, got %s", f.SlowProxyID, reserved.Proxy.ID)
	}

	allocationID := uuid.New().String()
	if err := db.Create(&domain.CustomerProxyAllocation{ID: allocationID, CustomerID: "customer-2", SubscriptionID: "sub-1", ProxyID: f.FastProxyID, CredentialID: "cred-1", RotationMode: "rotating", Country: "Vietnam", Status: "active", CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("allocation: %v", err)
	}
	rotated, err := svc.RotateAllocation("customer-2", allocationID, "manual")
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated.Proxy.ID == f.FastProxyID {
		t.Fatalf("expected rotation to choose another proxy")
	}
	failedOver, err := svc.FailoverAllocation("customer-2", allocationID)
	if err != nil {
		t.Fatalf("failover: %v", err)
	}
	if failedOver.Proxy.ID == "" {
		t.Fatalf("expected failover proxy")
	}
}

func TestRoutingDashboardHAScore(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()
	seedRoutingFixture(t, db)
	svc := NewService(db)
	data, err := svc.Dashboard()
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if data["ha_score"].(int) <= 0 {
		t.Fatalf("expected positive HA score: %+v", data)
	}
}

type routingFixture struct {
	FastProxyID string
	SlowProxyID string
}

func testDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	db, err := repository.NewSQLiteDB(t.TempDir() + "/routing.db")
	if err != nil {
		t.Fatalf("db init: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db handle: %v", err)
	}
	return db, func() { _ = sqlDB.Close() }
}

func seedRoutingFixture(t *testing.T, db *gorm.DB) routingFixture {
	t.Helper()
	now := time.Now()
	agents := []domain.Agent{
		{ID: "agent-vn-1", Name: "VN-01", Hostname: "vn-01", IPAddress: "10.0.0.1", Status: "healthy", LastHeartbeatAt: now, CreatedAt: now, UpdatedAt: now},
		{ID: "agent-vn-2", Name: "VN-02", Hostname: "vn-02", IPAddress: "10.0.0.2", Status: "healthy", LastHeartbeatAt: now, CreatedAt: now, UpdatedAt: now},
	}
	for _, agent := range agents {
		if err := db.Create(&agent).Error; err != nil {
			t.Fatalf("agent seed: %v", err)
		}
	}
	nodes := []domain.VPNNode{
		{ID: "node-vn-1", Name: "VNPT", Provider: "expressvpn", Type: "expressvpn", Status: "connected", Country: "Vietnam", Region: "North", ISP: "VNPT", ASN: "7552", AgentID: "agent-vn-1", CreatedAt: now, UpdatedAt: now},
		{ID: "node-vn-2", Name: "FPT", Provider: "expressvpn", Type: "expressvpn", Status: "connected", Country: "Vietnam", Region: "South", ISP: "FPT", ASN: "18403", AgentID: "agent-vn-2", CreatedAt: now, UpdatedAt: now},
	}
	for _, node := range nodes {
		if err := db.Create(&node).Error; err != nil {
			t.Fatalf("node seed: %v", err)
		}
	}
	proxies := []domain.Proxy{
		{ID: "proxy-fast", VPNNodeID: "node-vn-1", Port: 21001, Type: "socks5", Status: "running", BindIP: "127.0.0.1", Host: "127.0.0.1", Country: "Vietnam", Region: "North", AgentID: "agent-vn-1", CreatedAt: now, UpdatedAt: now},
		{ID: "proxy-slow", VPNNodeID: "node-vn-2", Port: 21002, Type: "socks5", Status: "running", BindIP: "127.0.0.1", Host: "127.0.0.1", Country: "Vietnam", Region: "South", AgentID: "agent-vn-2", CreatedAt: now, UpdatedAt: now},
	}
	for _, proxy := range proxies {
		if err := db.Create(&proxy).Error; err != nil {
			t.Fatalf("proxy seed: %v", err)
		}
	}
	metrics := []domain.HealthMetric{
		{TargetID: "proxy-fast", TargetType: "proxy", LatencyMs: 25, Status: "online", CheckedAt: now},
		{TargetID: "proxy-fast", TargetType: "proxy", LatencyMs: 30, Status: "online", CheckedAt: now.Add(-time.Minute)},
		{TargetID: "proxy-slow", TargetType: "proxy", LatencyMs: 240, Status: "online", CheckedAt: now},
		{TargetID: "proxy-slow", TargetType: "proxy", LatencyMs: 300, Status: "offline", CheckedAt: now.Add(-time.Minute)},
	}
	for _, metric := range metrics {
		if err := db.Create(&metric).Error; err != nil {
			t.Fatalf("metric seed: %v", err)
		}
	}
	return routingFixture{FastProxyID: "proxy-fast", SlowProxyID: "proxy-slow"}
}
