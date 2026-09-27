package loans

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"banking-service/internal/accounts"
	"banking-service/internal/money"
	"banking-service/internal/observability"
	"banking-service/internal/response"
)

// CustomerGetter loads a customer; *accounts.Repository satisfies it.
type CustomerGetter interface {
	GetByID(ctx context.Context, id string) (*accounts.Customer, error)
}

type EvaluateRequest struct {
	CustomerID string      `json:"customer_id" binding:"required,max=64" example:"customer-001"`
	LoanAmount json.Number `json:"loan_amount" binding:"required" swaggertype:"number" example:"500000"`
}

type EvaluateResult struct {
	CustomerID        string      `json:"customer_id" example:"customer-001"`
	Approved          bool        `json:"approved" example:"true"`
	CreditScore       int         `json:"credit_score" example:"742"`
	Reason            string      `json:"reason" example:"ELIGIBLE" enums:"ELIGIBLE,LOW_CREDIT_SCORE,IDENTITY_NOT_VERIFIED,AMOUNT_EXCEEDS_LIMIT"`
	MaxEligibleAmount json.Number `json:"max_eligible_amount" swaggertype:"number" example:"540727.75"`
}

type Handler struct {
	customers CustomerGetter
}

func NewHandler(customers CustomerGetter) *Handler {
	return &Handler{customers: customers}
}

// RegisterRoutes mounts the handler; middleware (e.g. rate limiting) runs first.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	rg.POST("/loans/evaluate", append(middleware, h.Evaluate)...)
}

// Evaluate godoc
//
//	@Summary		Evaluate loan eligibility
//	@Description	Applies deterministic lending rules (no LLM). Rules, first failure wins: credit score >= 650 (LOW_CREDIT_SCORE); 2FA verified (IDENTITY_NOT_VERIFIED); amount <= min(balance x multiplier, ₹1 crore) where the multiplier is 10x for score 750+, 5x for 700-749, 2x for 650-699 (AMOUNT_EXCEEDS_LIMIT).
//	@Tags			loans
//	@Accept			json
//	@Produce		json
//	@Param			request	body		EvaluateRequest							true	"Loan request"
//	@Success		200		{object}	response.Response{data=EvaluateResult}	"Evaluated (approved or rejected)"
//	@Failure		400		{object}	response.Response
//	@Failure		404		{object}	response.Response
//	@Failure		429		{object}	response.Response	"Rate limit exceeded; see Retry-After header"
//	@Failure		500		{object}	response.Response
//	@Router			/api/v1/loans/evaluate [post]
func (h *Handler) Evaluate(c *gin.Context) {
	var req EvaluateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request body: customer_id and loan_amount are required", nil)
		return
	}
	if !money.Valid(req.LoanAmount.String()) {
		response.BadRequest(c, "loan_amount must be a positive number with at most 2 decimal places", nil)
		return
	}

	customer, err := h.customers.GetByID(c.Request.Context(), req.CustomerID)
	if errors.Is(err, accounts.ErrNotFound) {
		response.NotFound(c, "Account not found", nil)
		return
	}
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "evaluate loan failed", "error", err)
		response.InternalServerError(c, "", nil)
		return
	}

	balance, ok := money.Parse(customer.Balance)
	if !ok {
		slog.ErrorContext(c.Request.Context(), "evaluate loan: invalid stored balance", "customer_id", customer.CustomerID)
		response.InternalServerError(c, "", nil)
		return
	}
	amount, _ := money.Parse(req.LoanAmount.String())

	d := Evaluate(customer.CreditScore, customer.Is2FAVerified, balance, amount)
	observability.LoanEvaluations.WithLabelValues(d.Reason).Inc()
	response.Success(c, "Loan evaluated", EvaluateResult{
		CustomerID:        customer.CustomerID,
		Approved:          d.Approved,
		CreditScore:       customer.CreditScore,
		Reason:            d.Reason,
		MaxEligibleAmount: json.Number(d.MaxEligibleAmount),
	})
}
