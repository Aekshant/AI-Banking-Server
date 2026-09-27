package ratelimit

import (
	"context"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"banking-service/internal/observability"
	"banking-service/internal/response"
)

// Timeout bounds each Redis call so a slow Redis cannot stall requests.
const Timeout = 100 * time.Millisecond

// Middleware limits requests per client IP using rule. Each rule has its own
// bucket, so for example reads never use up the transfer budget.
//
// If the limiter fails (Redis down or slow), the request is allowed and the
// error logged: an outage of the rate limiter must not take banking down.
func Middleware(limiter Limiter, rule Rule) gin.HandlerFunc {
	limit := strconv.Itoa(rule.Capacity)

	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), Timeout)
		defer cancel()

		res, err := limiter.Allow(ctx, rule.Name+":"+c.ClientIP(), rule)
		if err != nil {
			observability.RateLimitDecisions.WithLabelValues(rule.Name, "error").Inc()
			slog.WarnContext(c.Request.Context(), "rate limiter unavailable, allowing request", "rule", rule.Name, "error", err)
			c.Next()
			return
		}

		c.Header("X-RateLimit-Limit", limit)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))

		if !res.Allowed {
			observability.RateLimitDecisions.WithLabelValues(rule.Name, "rejected").Inc()
			seconds := int(math.Ceil(res.RetryAfter.Seconds()))
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			response.TooManyRequests(c, "", gin.H{"retry_after_seconds": seconds})
			c.Abort()
			return
		}
		observability.RateLimitDecisions.WithLabelValues(rule.Name, "allowed").Inc()
		c.Next()
	}
}
