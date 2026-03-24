package cmd

import (
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"

	v1 "github.com/clawhost/clawhost/handler/api/v1"
	"github.com/clawhost/clawhost/handler/proxy"
	authmw "github.com/clawhost/clawhost/middleware"
	"github.com/clawhost/clawhost/service/k8s"
	"github.com/clawhost/clawhost/web"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the API server",
	Run: func(cmd *cobra.Command, args []string) {
		if err := initConfig(); err != nil {
			log.Fatalf("init config failed: %v", err)
		}

		if err := k8s.InitClient(); err != nil {
			log.Fatalf("init k8s client failed: %v", err)
		}

		startServer()
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}

func startServer() {
	e := echo.New()

	// Get API domain to exclude from subdomain routing
	apiDomain := viper.GetString("domain.api_domain")

	// Subdomain routing middleware (must run BEFORE routing with e.Pre)
	// {bot-id}.any-domain/* -> /proxy/{bot-id}/*
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			host := c.Request().Host
			// Remove port if present
			if idx := strings.Index(host, ":"); idx > 0 {
				host = host[:idx]
			}

			// Skip IP addresses (e.g., K8s health checks via pod IP)
			if net.ParseIP(host) != nil {
				return next(c)
			}

			// Skip if this is the API domain itself (no subdomain)
			if host == apiDomain {
				return next(c)
			}

			// Extract first subdomain segment as bot ID
			if dotIdx := strings.Index(host, "."); dotIdx > 0 {
				botID := host[:dotIdx]
				if botID != "" {
					path := c.Request().URL.Path
					c.Request().URL.Path = "/proxy/" + botID + path
					return next(c)
				}
			}

			return next(c)
		}
	})

	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// API routes: /bot/api/v1/*
	api := e.Group("/bot/api/v1")
	api.Use(authmw.BearerAuth()) // Bearer token authentication
	{
		// Bot collection routes (no ownership check needed)
		api.POST("/bots", v1.CreateBot)
		api.GET("/bots", v1.ListBots)
	}

	// Bot instance routes: require ownership validation
	botAPI := api.Group("/bots/:id")
	botAPI.Use(authmw.BotOwnerAuth()) // Verify authenticated app owns the bot
	{
		// Bot CRUD
		botAPI.GET("", v1.GetBot)
		botAPI.PUT("", v1.UpdateBot)
		botAPI.DELETE("", v1.DeleteBot)

		// Bot lifecycle
		botAPI.POST("/start", v1.StartBot)
		botAPI.POST("/stop", v1.StopBot)
		botAPI.POST("/restart", v1.RestartBot)
		botAPI.GET("/status", v1.GetBotStatus)
		botAPI.GET("/connect", v1.GetBotConnect)
		botAPI.POST("/reset-token", v1.ResetBotToken)

		// Skills management
		botAPI.GET("/skills", v1.ListSkills)
		botAPI.PUT("/skills/:name", v1.UpdateSkill)
		botAPI.DELETE("/skills/:name", v1.DeleteSkill)

		// Channels management (IM integrations)
		botAPI.POST("/channels", v1.AddChannel)
		botAPI.GET("/channels", v1.ListChannels)
		botAPI.DELETE("/channels/:channel", v1.RemoveChannel)

		// Channel pairing management
		botAPI.GET("/channels/:channel/pairing", v1.ListChannelPairingRequests)
		botAPI.POST("/channels/:channel/pairing/approve", v1.ApproveChannelPairing)
		botAPI.POST("/channels/:channel/pairing/revoke", v1.RevokeChannelPairing)
		botAPI.GET("/channels/:channel/pairing/users", v1.GetChannelPairedUsers)

		// WeChat channel management (QR code login + multi-account)
		botAPI.POST("/channels/wechat/login", v1.WechatLoginStart)
		botAPI.GET("/channels/wechat/login/status", v1.WechatLoginStatus)
		botAPI.GET("/channels/wechat/accounts", v1.WechatListAccounts)
		botAPI.DELETE("/channels/wechat/accounts/:account_id", v1.WechatRemoveAccount)

		// Device pairing management
		botAPI.GET("/devices", v1.ListDevices)
		botAPI.POST("/devices/:request_id/approve", v1.ApproveDevice)
		botAPI.DELETE("/devices/:device_id", v1.RevokeDevice)

		// Model providers management
		botAPI.GET("/config/models", v1.ListModelProviders)
		botAPI.POST("/config/models", v1.AddModelProvider)
		botAPI.GET("/config/models/:provider", v1.GetModelProvider)
		botAPI.PUT("/config/models/:provider", v1.UpdateModelProvider)
		botAPI.DELETE("/config/models/:provider", v1.DeleteModelProvider)

		// Agent defaults management
		botAPI.GET("/config/defaults", v1.GetAgentDefaults)
		botAPI.PUT("/config/defaults", v1.SetAgentDefaults)

		// Raw openclaw.json config (read/write from running pod)
		botAPI.GET("/config/raw", v1.GetBotRawConfig)
		botAPI.PUT("/config/raw", v1.UpdateBotRawConfig)
	}

	// Admin API routes: /bot/api/v1/admin/* (requires admin token)
	admin := e.Group("/bot/api/v1/admin")
	admin.Use(authmw.AdminAuth())
	{
		// Token verification
		admin.GET("/verify", func(c echo.Context) error {
			return c.JSON(200, map[string]interface{}{"code": 0, "message": "success"})
		})

		// Global config (for admin UI)
		admin.GET("/config", func(c echo.Context) error {
			return c.JSON(200, map[string]interface{}{
				"code":    0,
				"message": "success",
				"data": map[string]interface{}{
					"bot_domain_template": viper.GetString("domain.bot_domain_template"),
				},
			})
		})

		// App management
		admin.POST("/apps", v1.CreateApp)
		admin.GET("/apps", v1.ListApps)
		admin.GET("/apps/:id", v1.GetApp)
		admin.PUT("/apps/:id", v1.UpdateApp)
		admin.DELETE("/apps/:id", v1.DeleteApp)
		admin.POST("/apps/:id/reset-token", v1.ResetAppToken)

		// Bot management (admin)
		admin.POST("/bots", v1.AdminCreateBot)
		admin.GET("/bots", v1.AdminListBots)
		admin.POST("/bots/:id/start", v1.AdminStartBot)
		admin.POST("/bots/:id/stop", v1.AdminStopBot)
		admin.DELETE("/bots/:id", v1.AdminDeleteBot)

		// Bot upgrade management
		admin.POST("/bots/upgrade", v1.UpgradeAllBots)
		admin.POST("/bots/:id/upgrade", v1.UpgradeBot)

		// Bot restart management (full pod spec rebuild)
		admin.POST("/bots/restart", v1.RestartAllBots)
	}

	// Health check
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(200, map[string]string{"status": "ok"})
	})

	// Admin UI (served from embedded Next.js static export)
	adminFS, err := fs.Sub(web.AdminFS, "admin/out")
	if err != nil {
		log.Printf("Warning: admin UI not available: %v", err)
	} else {
		adminHandler := http.FileServer(http.FS(adminFS))
		e.GET("/admin/*", echo.WrapHandler(http.StripPrefix("/admin", adminHandler)))
		e.GET("/admin", func(c echo.Context) error {
			return c.Redirect(301, "/admin/")
		})
	}

	// Bot proxy routes (for {bot_id}.clawhost.ai/*)
	e.Any("/proxy/:bot_id", proxy.ProxyToBot)
	e.Any("/proxy/:bot_id/*", proxy.ProxyToBot)

	port := viper.GetInt("server.port")
	if port == 0 {
		port = 8080
	}

	log.Printf("Starting server on port %d", port)
	log.Printf("Admin UI: http://localhost:%d/admin", port)
	e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", port)))
}
