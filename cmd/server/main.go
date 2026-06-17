package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"

	"vpn-to-proxy/internal/abuse"
	"vpn-to-proxy/internal/api"
	"vpn-to-proxy/internal/billing"
	"vpn-to-proxy/internal/customer"
	"vpn-to-proxy/internal/domain"
	"vpn-to-proxy/internal/event"
	"vpn-to-proxy/internal/health"
	"vpn-to-proxy/internal/proxy"
	"vpn-to-proxy/internal/repository"
	"vpn-to-proxy/internal/routing"
	"vpn-to-proxy/internal/security"
	"vpn-to-proxy/internal/vpn"
)

func main() {
	dbPath := flag.String("db", "vpn_to_proxy.db", "Path to SQLite database")
	port := flag.String("port", "8080", "Port to run backend API on")
	flag.Parse()

	log.Printf("Starting VPN to Proxy Management Platform Backend on port %s...", *port)

	// 1. Initialize SQLite database
	resolvedDBPath, err := resolveDBPath(*dbPath)
	if err != nil {
		log.Fatalf("Failed to resolve database path: %v", err)
	}
	if resolvedDBPath != *dbPath {
		log.Printf("Database path %q is not writable, using %q instead", *dbPath, resolvedDBPath)
	}

	db, activeDBPath, err := openSQLiteDB(resolvedDBPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	if activeDBPath != resolvedDBPath {
		log.Printf("Primary database %q failed write validation, using fallback %q", resolvedDBPath, activeDBPath)
	}
	log.Println("SQLite database initialized successfully")

	// Check security encryption key
	if _, err := security.GetSecretKey(); err != nil {
		log.Fatalf("Security initialization failed: %v", err)
	}

	// 2. Initialize Repositories
	vpnRepo := repository.NewVPNNodeRepository(db)
	proxyRepo := repository.NewProxyRepository(db)
	auditRepo := repository.NewAuditLogRepository(db)
	metricRepo := repository.NewHealthMetricRepository(db)
	credRepo := repository.NewVPNCredentialRepository(db)
	agentRepo := repository.NewAgentRepository(db)
	sessionRepo := repository.NewExpressVPNSessionRepository(db)

	hbRepo := repository.NewAgentHeartbeatRepository(db)
	allocRepo := repository.NewPortAllocationRepository(db)
	policyRepo := repository.NewRotationPolicyRepository(db)
	validationRepo := repository.NewExpressVPNValidationRepository(db)
	systemMetricRepo := repository.NewSystemMetricRepository(db)
	agentCredRepo := repository.NewAgentCredentialRepository(db)
	agentCmdRepo := repository.NewAgentCommandRepository(db)

	_ = allocRepo
	_ = policyRepo

	// Create default agent if it doesn't exist
	if _, err := agentRepo.GetByID("local-agent"); err != nil {
		err = agentRepo.Create(&domain.Agent{
			ID:              "local-agent",
			Name:            "local-agent",
			Hostname:        "localhost",
			IPAddress:       "127.0.0.1",
			OS:              "local",
			Version:         "v1.0.0",
			Status:          "healthy",
			LastHeartbeatAt: time.Now(),
			VPNCount:        0,
			ProxyCount:      0,
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		})
		if err != nil {
			log.Fatalf("Failed to initialize local agent: %v", err)
		}
		log.Println("Default agent 'local-agent' initialized successfully")
	}

	// Create initial audit log
	auditRepo.Create(&domain.AuditLog{
		Action:    "SYSTEM_BOOT",
		Details:   "VPN-to-Proxy service started",
		Timestamp: time.Now(),
	})

	// 3. Initialize Managers & Services
	vpnMgr := vpn.NewVpnManager(vpnRepo, auditRepo, credRepo, agentRepo, sessionRepo)
	proxyMgr := proxy.NewProxyManager(proxyRepo, vpnRepo, auditRepo, vpnMgr)
	customerService := customer.NewService(db, proxyRepo, auditRepo)
	customerService.EnsureDefaultPlans()
	billingService := billing.NewService(db, activeDBPath, billing.MockPaymentProvider{})
	billingService.EnsureDefaultPlans()
	customerService.SetSubscriptionEnforcer(billingService)
	abuseService := abuse.NewService(db)
	abuseService.EnsureDefaultRules()
	routingService := routing.NewService(db)
	routingService.EnsureDefaultPools()
	customerService.SetRoutingSelector(routingService)
	proxy.SetCustomerAuthValidator(abuseService.ValidateProxyCredential)
	proxy.SetConnectionGuard(abuseService.GuardConnection)
	proxy.SetUsageRecorder(abuseService.RecordUsage)

	heartbeatService := health.NewAgentHeartbeatService(agentRepo, hbRepo)
	metricsService := health.NewSystemMetricsService(systemMetricRepo, vpnRepo, proxyRepo)
	validationService := vpn.NewExpressVPNValidationService(vpnRepo, validationRepo, vpnMgr)
	healthMonitor := health.NewHealthMonitorService(proxyRepo, vpnRepo, agentRepo, metricRepo, auditRepo)
	eventBus := event.GetBus()
	commandBus := event.NewCommandBus(agentCmdRepo, agentRepo, eventBus)
	dashboardWS := api.NewDashboardWSGateway(eventBus)
	agentWS := api.NewAgentWSGateway(db, agentCredRepo, agentRepo, commandBus, heartbeatService, eventBus)

	// 4. Restore previously running proxies
	log.Println("Restoring active proxies...")
	proxyMgr.RestoreProxies()

	// 5. Start Background Services
	log.Println("Starting background services...")
	ctxBg := context.Background()
	heartbeatService.Start(ctxBg)
	metricsService.Start(ctxBg)
	healthMonitor.Start(ctxBg)
	proxyMgr.StartExpiryMonitor(ctxBg, time.Minute)

	// Record initial heartbeat for local agent
	heartbeatService.RecordHeartbeat("local-agent", 0, 0, 0, 0)

	// 6. Setup Gin Router & Server
	handler := api.NewHandler(
		vpnRepo,
		proxyRepo,
		auditRepo,
		metricRepo,
		credRepo,
		agentRepo,
		agentCredRepo,
		agentCmdRepo,
		sessionRepo,
		vpnMgr,
		proxyMgr,
		heartbeatService,
		metricsService,
		validationService,
		healthMonitor,
		customerService,
		billingService,
		abuseService,
		routingService,
		dashboardWS,
		agentWS,
		commandBus,
		eventBus,
	)
	router := api.NewRouter(handler)

	srv := &http.Server{
		Addr:    ":" + *port,
		Handler: router,
	}

	// 7. Graceful Shutdown listener
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Printf("API listening on http://localhost:%s", *port)

	// Wait for interrupt signal to gracefully shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server gracefully...")

	// Create shutdown context with 5s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	// Stop background services
	heartbeatService.Stop()
	metricsService.Stop()
	healthMonitor.Stop()

	// Stop all running proxies
	log.Println("Stopping active proxies...")
	proxies, err := proxyRepo.List()
	if err == nil {
		for _, prxy := range proxies {
			if prxy.Status == "running" {
				proxyMgr.StopProxy(prxy.ID)
			}
		}
	}

	log.Println("Server exiting")
}

func resolveDBPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "vpn_to_proxy.db"
	}

	if err := ensureWritableSQLitePath(path); err == nil {
		return path, nil
	}

	fallbackDir := filepath.Join(os.TempDir(), "vpn-to-proxy")
	if err := os.MkdirAll(fallbackDir, 0o755); err != nil {
		return "", err
	}

	fallbackPath := filepath.Join(fallbackDir, filepath.Base(path))
	if err := ensureWritableSQLitePath(fallbackPath); err != nil {
		return "", fmt.Errorf("primary path %q is unavailable and fallback %q also failed: %w", path, fallbackPath, err)
	}

	return fallbackPath, nil
}

func openSQLiteDB(path string) (*gorm.DB, string, error) {
	db, err := repository.NewSQLiteDB(path)
	if err == nil {
		return db, path, nil
	}

	fallbackPath, fallbackErr := fallbackDBPath(path)
	if fallbackErr != nil {
		return nil, "", fmt.Errorf("primary database %q failed: %w; fallback path failed: %v", path, err, fallbackErr)
	}
	if fallbackPath == path {
		return nil, "", err
	}

	db, fallbackErr = repository.NewSQLiteDB(fallbackPath)
	if fallbackErr != nil {
		return nil, "", fmt.Errorf("primary database %q failed: %w; fallback database %q failed: %v", path, err, fallbackPath, fallbackErr)
	}
	return db, fallbackPath, nil
}

func fallbackDBPath(path string) (string, error) {
	fallbackDir := filepath.Join(os.TempDir(), "vpn-to-proxy")
	if err := os.MkdirAll(fallbackDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(fallbackDir, filepath.Base(path)), nil
}

func ensureWritableSQLitePath(path string) error {
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
