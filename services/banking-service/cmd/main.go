package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	_ "banking-service/docs"
	"banking-service/internal/accounts"
	"banking-service/internal/config"
	"banking-service/internal/db"
	"banking-service/internal/health"
	"banking-service/internal/loans"
	"banking-service/internal/observability"
	"banking-service/internal/ratelimit"
	"banking-service/internal/response"
	"banking-service/internal/transactions"
)

//	@title			Banking Service API
//	@version		1.0
//	@description	Core banking API for the bank agent platform. PII is masked at this layer.
//	@BasePath		/

func main() {
	// `banking-service healthcheck` is used by the Docker HEALTHCHECK; the
	// distroless runtime image has no curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger, err := observability.NewLogger(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	if err != nil {
		return err
	}
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := observability.SetupTracing(ctx)
	if err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(flushCtx); err != nil {
			slog.Warn("flush traces", "error", err)
		}
	}()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to postgres: %w", err)
	}
	defer pool.Close()
	observability.RegisterDBPool(pool)

	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		DialTimeout:  ratelimit.Timeout,
		ReadTimeout:  ratelimit.Timeout,
		WriteTimeout: ratelimit.Timeout,
	})
	defer rdb.Close()
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		return fmt.Errorf("redis tracing: %w", err)
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unreachable; rate limiting will fail open", "addr", cfg.RedisAddr, "error", err)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router(cfg, pool, rdb),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("banking-service listening", "port", cfg.Port, "version", observability.Version)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}

func router(cfg config.Config, pool *pgxpool.Pool, rdb *redis.Client) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		slog.Error("invalid TRUSTED_PROXIES", "error", err)
		os.Exit(1)
	}
	r.HandleMethodNotAllowed = true

	// Order matters: tracing first so every later stage sees the span;
	// recovery last so a panic is still logged and counted as a 500.
	r.Use(
		otelgin.Middleware(observability.ServiceName, otelgin.WithFilter(func(req *http.Request) bool {
			return req.URL.Path != "/health" && req.URL.Path != "/metrics"
		})),
		observability.RequestIDMiddleware(),
		observability.AccessLog(),
		observability.HTTPMetrics(),
		gin.CustomRecovery(func(c *gin.Context, err any) {
			slog.ErrorContext(c.Request.Context(), "panic", "error", err)
			response.InternalServerError(c, "", nil)
		}),
	)
	r.NoRoute(func(c *gin.Context) { response.NotFound(c, "Route not found", nil) })
	r.NoMethod(func(c *gin.Context) { response.MethodNotAllowed(c, "", nil) })

	r.GET("/health", health.NewHandler(pool, rdb).Check)
	r.GET("/metrics", gin.WrapH(observability.MetricsHandler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	api := r.Group("/api/v1")
	limit := rateLimiter(cfg, rdb)

	accountsRepo := accounts.NewRepository(pool)
	accounts.NewHandler(accountsRepo).RegisterRoutes(api, limit(cfg.RateLimits.Read)...)
	loans.NewHandler(accountsRepo).RegisterRoutes(api, limit(cfg.RateLimits.Loan)...)
	transactions.NewHandler(transactions.NewRepository(pool)).RegisterRoutes(api, limit(cfg.RateLimits.Transfer)...)
	return r
}

// rateLimiter returns a function giving the middleware for a rule: a Redis
// token bucket, or nothing when RATE_LIMIT_ENABLED=false.
func rateLimiter(cfg config.Config, rdb *redis.Client) func(ratelimit.Rule) []gin.HandlerFunc {
	if !cfg.RateLimitEnabled {
		slog.Warn("rate limiting disabled")
		return func(ratelimit.Rule) []gin.HandlerFunc { return nil }
	}
	limiter := ratelimit.NewRedisLimiter(rdb)
	return func(rule ratelimit.Rule) []gin.HandlerFunc {
		return []gin.HandlerFunc{ratelimit.Middleware(limiter, rule)}
	}
}

// healthcheck calls the local /health endpoint and returns a process exit code.
func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8001"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}
