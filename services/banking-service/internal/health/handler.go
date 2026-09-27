package health

import (
	"context"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"banking-service/internal/observability"
	"banking-service/internal/response"
)

type Handler struct {
	db    *pgxpool.Pool
	redis *redis.Client
}

type Status struct {
	Database string `json:"database" example:"up"`
	// Redis only backs rate limiting, which fails open, so "down" here
	// degrades the service but does not make it unhealthy.
	Redis string `json:"redis" example:"up"`
}

func NewHandler(db *pgxpool.Pool, redis *redis.Client) *Handler {
	return &Handler{db: db, redis: redis}
}

// Check godoc
//
//	@Summary		Health check
//	@Description	Reports service health. Returns 503 if Postgres is down. Redis being down is reported but still returns 200, because rate limiting fails open.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	response.Response{data=Status}
//	@Failure		503	{object}	response.Response{data=Status}
//	@Router			/health [get]
func (h *Handler) Check(c *gin.Context) {
	ctx, cancel := context.WithTimeout(observability.WithoutTracing(c.Request.Context()), 2*time.Second)
	defer cancel()

	status := Status{Database: "up", Redis: "up"}
	if err := h.redis.Ping(ctx).Err(); err != nil {
		slog.WarnContext(ctx, "health check: redis ping failed", "error", err)
		status.Redis = "down"
	}

	if err := h.db.Ping(ctx); err != nil {
		slog.ErrorContext(ctx, "health check: database ping failed", "error", err)
		status.Database = "down"
		response.ServiceUnavailable(c, "Database unavailable", status)
		return
	}

	response.Success(c, "Service is healthy", status)
}
