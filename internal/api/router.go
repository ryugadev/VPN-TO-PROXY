package api

import (
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

func NewRouter(h *Handler) *gin.Engine {
	r := gin.Default()

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})
	r.Use(h.RateLimitMiddleware())

	// API Routes Group
	api := r.Group("/api")
	{
		api.GET("/vpns", h.ListVPNs)
		api.POST("/vpns", h.CreateVPN)
		api.POST("/vpns/:id/connect", h.ConnectVPN)
		api.POST("/vpns/:id/disconnect", h.DisconnectVPN)
		api.DELETE("/vpns/:id", h.DeleteVPN)

		// ExpressVPN specific endpoints
		api.POST("/vpn/credentials", h.CreateCredential)
		api.GET("/vpn/credentials", h.ListCredentials)
		api.GET("/vpn/expressvpn/locations", h.ListLocations)
		api.POST("/vpn/expressvpn/nodes", h.CreateExpressVPNNode)
		api.GET("/vpn/:id/status", h.GetVPNStatus)
		api.POST("/vpn/:id/connect-and-create-proxy", h.ConnectAndCreateProxy)

		api.GET("/proxies", h.ListProxies)
		api.POST("/proxies", h.CreateProxy)
		api.POST("/proxies/:id/start", h.StartProxy)
		api.POST("/proxies/:id/stop", h.StopProxy)
		api.DELETE("/proxies/:id", h.DeleteProxy)
		api.GET("/proxies/:id/metrics", h.GetProxyMetrics)
		api.GET("/pools", h.ListPools)
		api.GET("/pools/:country", h.GetPool)
		api.GET("/pools/:country/health", h.GetPoolHealth)

		api.GET("/system/stats", h.GetSystemStats)
		api.GET("/system/logs", h.GetLogs)

		// Agent Registry
		api.POST("/agents/register", h.RegisterAgentWithToken)
		api.GET("/agents", h.ListAgents)
		api.GET("/agents/:id", h.GetAgent)
		api.DELETE("/agents/:id", h.DeleteAgent)
		api.POST("/agents/:id/heartbeat", h.ReceiveHeartbeat)
		api.POST("/agents/:id/rotate-token", h.RotateToken)
		api.POST("/agents/:id/revoke", h.RevokeAgent)
		api.POST("/agents/:id/commands", h.CreateAgentCommand)
		api.GET("/agents/:id/commands", h.ListAgentCommands)
		api.GET("/commands", h.ListCommands)
		api.GET("/commands/:id", h.GetCommandStatus)
		api.GET("/audit/export", h.ExportAuditLog)

		api.POST("/customer/register", h.CustomerRegister)
		api.POST("/customer/login", h.CustomerLogin)
		api.GET("/customer/plans", h.CustomerListPlans)
		api.GET("/customer/proxies", h.CustomerListProxies)
		api.POST("/customer/proxies/allocate", h.CustomerAllocateProxy)
		api.POST("/customer/proxies/:id/rotate-credential", h.CustomerRotateCredential)
		api.POST("/customer/proxies/:id/rotate", h.CustomerRotateProxy)
		api.DELETE("/customer/proxies/:id", h.CustomerReleaseProxy)
		api.GET("/customer/proxies/export", h.CustomerExportProxies)
		api.GET("/customer/usage", h.CustomerUsage)
		api.POST("/customer/api-keys", h.CustomerCreateAPIKey)
		api.DELETE("/customer/api-keys/:id", h.CustomerDeleteAPIKey)

		api.POST("/admin/plans", h.AdminCreatePlan)
		api.POST("/admin/customers/:id/subscriptions", h.AdminCreateSubscription)
		api.GET("/admin/billing/overview", h.AdminBillingOverview)
		api.GET("/admin/metrics", h.AdminProductionMetrics)
		api.GET("/admin/billing/plans", h.AdminListCommercialPlans)
		api.POST("/admin/billing/plans", h.AdminCreateCommercialPlan)
		api.PUT("/admin/billing/plans/:id", h.AdminUpdateCommercialPlan)
		api.POST("/admin/billing/subscriptions", h.AdminCreateCommercialSubscription)
		api.POST("/admin/billing/subscriptions/:id/status", h.AdminUpdateSubscriptionStatus)
		api.POST("/admin/billing/invoices", h.AdminGenerateInvoice)
		api.POST("/admin/billing/invoices/:id/paid", h.AdminMarkInvoicePaid)
		api.POST("/admin/billing/invoices/:id/verify", h.AdminVerifyInvoicePayment)
		api.POST("/admin/billing/invoices/:id/refund", h.AdminRefundInvoice)
		api.GET("/admin/billing/invoices/:id/payment-status", h.AdminInvoicePaymentStatus)
		api.POST("/admin/customers/:id/suspend", h.AdminSuspendCustomer)
		api.POST("/admin/customers/:id/activate", h.AdminActivateCustomer)
		api.GET("/admin/audit-events", h.AdminListAuditEvents)
		api.GET("/admin/backup/export", h.AdminExportBackup)
		api.GET("/customer/subscription/usage", h.CustomerSubscriptionUsage)
		api.GET("/customer/ip-whitelist", h.CustomerListIPWhitelist)
		api.POST("/customer/ip-whitelist", h.CustomerAddIPWhitelist)
		api.DELETE("/customer/ip-whitelist/:id", h.CustomerDeleteIPWhitelist)
		api.GET("/customer/security", h.CustomerSecurityDashboard)

		api.GET("/admin/blocked-targets", h.AdminListBlockedTargets)
		api.POST("/admin/blocked-targets", h.AdminAddBlockedTarget)
		api.DELETE("/admin/blocked-targets/:id", h.AdminDeleteBlockedTarget)
		api.GET("/admin/abuse/dashboard", h.AdminAbuseDashboard)
		api.POST("/admin/abuse/customers/:id/suspend", h.AdminAbuseSuspendCustomer)
		api.POST("/admin/abuse/customers/:id/unsuspend", h.AdminAbuseUnsuspendCustomer)
		api.POST("/admin/abuse/proxies/:id/suspend", h.AdminAbuseSuspendProxy)
		api.POST("/admin/abuse/credentials/:id/disable", h.AdminAbuseDisableCredential)
		api.POST("/admin/abuse/customers/:id/clear-risk", h.AdminAbuseClearRisk)
		api.GET("/admin/routing/dashboard", h.AdminRoutingDashboard)
		api.POST("/admin/routing/pools", h.AdminCreatePool)
		api.POST("/admin/routing/pools/sync", h.AdminSyncPools)
		api.POST("/admin/routing/select", h.AdminSelectProxy)
		api.POST("/admin/routing/allocations/:id/rotate", h.AdminRotateAllocation)
		api.POST("/admin/routing/allocations/:id/failover", h.AdminFailoverAllocation)
		api.POST("/admin/routing/reservations", h.AdminCreateReservation)

		// Validation
		api.POST("/vpn/:id/validate", h.ValidateVPN)

		// Prometheus metrics
		api.GET("/prometheus/metrics", h.GetPrometheusMetrics)
	}

	// WebSocket endpoints
	r.GET("/ws", func(c *gin.Context) {
		h.dashboardWS.Handler().ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/ws/dashboard", func(c *gin.Context) {
		h.dashboardWS.Handler().ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/ws/agent", func(c *gin.Context) {
		h.agentWS.Handler().ServeHTTP(c.Writer, c.Request)
	})

	// Serve React Frontend Static files if they exist
	webDistDir := "./webapp/dist"
	if _, err := os.Stat(webDistDir); err == nil {
		r.Static("/assets", filepath.Join(webDistDir, "assets"))
		r.StaticFile("/favicon.ico", filepath.Join(webDistDir, "favicon.ico"))

		// Fallback for SPA routing: serve index.html for all non-api routes
		r.NoRoute(func(c *gin.Context) {
			c.File(filepath.Join(webDistDir, "index.html"))
		})
	}

	return r
}
