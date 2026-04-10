package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mezamozg/meza-core/internal/api"
	"github.com/mezamozg/meza-core/internal/domain"
	"github.com/mezamozg/meza-core/internal/service"
)

func main() {
	cfg := loadConfig()
	store, err := service.NewMemoryStoreWithFile(cfg.DataPath)
	if err != nil {
		log.Fatalf("failed to initialize store: %v", err)
	}
	planner := buildPlanner(cfg, store.GetAISettings())
	server := api.NewServer(store, planner, api.AuthConfig{
		OperatorToken:    cfg.OperatorToken,
		BootstrapToken:   cfg.BootstrapToken,
		NodeToken:        cfg.NodeToken,
		AllowAnonymousUI: cfg.AllowAnonymousUI,
	})

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("meza-core listening on %s", cfg.Addr)
		log.Printf("persistence=%s", cfg.DataPath)
		log.Printf("ai_provider=%s", cfg.AIProvider)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server stopped: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("shutting down meza-core")
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("graceful shutdown failed: %v", err)
	}
}

type config struct {
	Addr             string
	DataPath         string
	OperatorToken    string
	BootstrapToken   string
	NodeToken        string
	AllowAnonymousUI bool
	AIProvider       string
	GeminiAPIKey     string
	GeminiModel      string
	GeminiBaseURL    string
}

func loadConfig() config {
	return config{
		Addr:             envOrDefault("MEZA_CORE_ADDR", ":8080"),
		DataPath:         envOrDefault("MEZA_CORE_DATA_PATH", "./data/state.json"),
		OperatorToken:    envOrDefault("MEZA_OPERATOR_TOKEN", "dev-operator-token"),
		BootstrapToken:   envOrDefault("MEZA_BOOTSTRAP_TOKEN", "dev-bootstrap-token"),
		NodeToken:        envOrDefault("MEZA_NODE_TOKEN", "dev-node-token"),
		AllowAnonymousUI: envOrDefault("MEZA_ALLOW_ANONYMOUS_UI", "false") == "true",
		AIProvider:       envOrDefault("MEZA_AI_PROVIDER", domain.DefaultAIProvider),
		GeminiAPIKey:     os.Getenv("MEZA_GEMINI_API_KEY"),
		GeminiModel:      envOrDefault("MEZA_GEMINI_MODEL", domain.DefaultGeminiModel),
		GeminiBaseURL:    envOrDefault("MEZA_GEMINI_BASE_URL", domain.DefaultGeminiBaseURL),
	}
}

func buildPlanner(cfg config, persisted domain.AISettings) service.Planner {
	planner := service.NewRuntimePlanner(service.RuntimePlannerConfig{
		Provider:      cfg.AIProvider,
		GeminiAPIKey:  cfg.GeminiAPIKey,
		GeminiModel:   cfg.GeminiModel,
		GeminiBaseURL: cfg.GeminiBaseURL,
		Timeout:       15 * time.Second,
		Fallback:      service.NewStubPlanner(),
	})

	if persisted.Provider != "" || persisted.GeminiAPIKey != "" || persisted.GeminiModel != "" || persisted.GeminiBaseURL != "" {
		planner.ApplySettings(persisted)
	}

	return planner
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
