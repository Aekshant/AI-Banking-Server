package transactions

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"

	"github.com/gin-gonic/gin"

	"banking-service/internal/money"
	"banking-service/internal/observability"
	"banking-service/internal/response"
)

type TransferRequest struct {
	FromAccount    string      `json:"from_account" binding:"required,max=64" example:"customer-001"`
	ToAccount      string      `json:"to_account" binding:"required,max=64" example:"customer-002"`
	Amount         json.Number `json:"amount" binding:"required" swaggertype:"number" example:"5000"`
	IdempotencyKey string      `json:"idempotency_key" binding:"required,max=128" example:"tx-123"`
}

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// RegisterRoutes mounts the handler; middleware (e.g. rate limiting) runs first.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	rg.POST("/accounts/transfer", append(middleware, h.Transfer)...)
}

// Transfer godoc
//
//	@Summary		Transfer funds
//	@Description	Moves money between two accounts atomically. Retrying with the same idempotency_key returns the original transaction instead of transferring again.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			request	body		TransferRequest						true	"Transfer details"
//	@Success		201		{object}	response.Response{data=Transaction}	"Transfer settled"
//	@Success		200		{object}	response.Response{data=Transaction}	"Duplicate request; original transaction returned"
//	@Failure		400		{object}	response.Response					"Invalid request or insufficient funds"
//	@Failure		404		{object}	response.Response					"Account not found"
//	@Failure		409		{object}	response.Response					"Idempotency key reused with different parameters"
//	@Failure		429		{object}	response.Response					"Rate limit exceeded; see Retry-After header"
//	@Failure		500		{object}	response.Response
//	@Router			/api/v1/accounts/transfer [post]
func (h *Handler) Transfer(c *gin.Context) {
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		observability.TransfersTotal.WithLabelValues("invalid").Inc()
		response.BadRequest(c, "Invalid request body: from_account, to_account, amount and idempotency_key are required", nil)
		return
	}
	if req.FromAccount == req.ToAccount {
		observability.TransfersTotal.WithLabelValues("invalid").Inc()
		response.BadRequest(c, "from_account and to_account must be different", nil)
		return
	}
	if !money.Valid(req.Amount.String()) {
		observability.TransfersTotal.WithLabelValues("invalid").Inc()
		response.BadRequest(c, "amount must be a positive number with at most 2 decimal places", nil)
		return
	}

	ctx := c.Request.Context()
	t, replayed, err := h.repo.Transfer(ctx, req.FromAccount, req.ToAccount, req.Amount.String(), req.IdempotencyKey)
	switch {
	case errors.Is(err, ErrAccountNotFound):
		observability.TransfersTotal.WithLabelValues("account_not_found").Inc()
		response.NotFound(c, "Account not found", nil)
	case errors.Is(err, ErrInsufficientFunds):
		observability.TransfersTotal.WithLabelValues("insufficient_funds").Inc()
		response.BadRequest(c, "Insufficient funds", nil)
	case errors.Is(err, ErrIdempotencyConflict):
		observability.TransfersTotal.WithLabelValues("idempotency_conflict").Inc()
		slog.WarnContext(ctx, "idempotency key reused for a different transfer", "idempotency_key", req.IdempotencyKey)
		response.Conflict(c, "idempotency_key was already used for a different transfer", nil)
	case err != nil:
		observability.TransfersTotal.WithLabelValues("error").Inc()
		slog.ErrorContext(ctx, "transfer failed", "error", err)
		response.InternalServerError(c, "", nil)
	case replayed:
		observability.TransfersTotal.WithLabelValues("replayed").Inc()
		slog.InfoContext(ctx, "transfer replayed", "transaction_id", t.TransactionID, "idempotency_key", req.IdempotencyKey)
		response.Success(c, "Duplicate request; returning original transaction", t)
	default:
		observability.TransfersTotal.WithLabelValues("settled").Inc()
		if amount, err := strconv.ParseFloat(t.Amount.String(), 64); err == nil {
			observability.TransferAmount.Add(amount) // float is fine for a metric; the ledger stays exact
		}
		slog.InfoContext(ctx, "transfer settled", "transaction_id", t.TransactionID,
			"from_account", req.FromAccount, "to_account", req.ToAccount, "amount", t.Amount.String())
		response.Created(c, "Transfer settled", t)
	}
}
