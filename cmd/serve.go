package cmd

import (
	"log"

	"github.com/allintech/github-sentry/config"
	"github.com/allintech/github-sentry/database"
	"github.com/allintech/github-sentry/http"
	"github.com/allintech/github-sentry/logger"
	"github.com/allintech/github-sentry/middleware"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the webhook server",
	Long:  `Start the GitHub webhook server that listens for push events and processes them.`,
	Run: func(cmd *cobra.Command, args []string) {
		runServer()
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServer() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
		return
	}

	// Initialize logger
	if err := logger.InitLogger(cfg.LogFolder); err != nil {
		log.Fatalf("failed to initialize logger: %v", err)
		return
	}
	defer logger.Close()

	// Initialize database
	if err := database.InitDB(cfg); err != nil {
		logger.LogError("failed to initialize database: %v", err)
		log.Fatalf("failed to initialize database: %v", err)
		return
	}
	defer database.Close()

	app := gin.Default()
	app.Use(gin.Recovery())
	app.Use(middleware.InjectMiddleware("config", cfg))
	api := app.Group("/tool/github-sentry")

	api.POST("/webhook", http.WebHook)
	api.GET("/health", http.HealthCheck)

	logger.LogInfo("starting server on %s", cfg.Addr)
	log.Printf("listening on %s", cfg.Addr)
	if err := app.Run(cfg.Addr); err != nil {
		logger.LogError("server error: %v", err)
		log.Fatal(err)
	}
}
