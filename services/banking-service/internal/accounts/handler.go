package accounts

import (
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"banking-service/internal/response"
)

// Profile is the client-facing view of a customer. PII is always masked.
type Profile struct {
	CustomerID    string      `json:"customer_id" example:"customer-001"`
	FullName      string      `json:"full_name" example:"Rahul Sharma"`
	PAN           string      `json:"pan" example:"XXXXX1234X"`
	Aadhaar       string      `json:"aadhaar" example:"XXXX XXXX 9012"`
	Balance       json.Number `json:"balance" swaggertype:"number" example:"125000.50"`
	CreditScore   int         `json:"credit_score" example:"742"`
	Is2FAVerified bool        `json:"is_2fa_verified" example:"true"`
}

func ToProfile(c *Customer) Profile {
	return Profile{
		CustomerID:    c.CustomerID,
		FullName:      c.FullName,
		PAN:           MaskPAN(c.PAN),
		Aadhaar:       MaskAadhaar(c.Aadhaar),
		Balance:       json.Number(c.Balance),
		CreditScore:   c.CreditScore,
		Is2FAVerified: c.Is2FAVerified,
	}
}

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

// RegisterRoutes mounts the handler; middleware (e.g. rate limiting) runs first.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, middleware ...gin.HandlerFunc) {
	rg.GET("/accounts/:id/profile", append(middleware, h.GetProfile)...)
}

// GetProfile godoc
//
//	@Summary		Get account profile
//	@Description	Returns a customer's profile. PAN and Aadhaar are masked by the API.
//	@Tags			accounts
//	@Produce		json
//	@Param			id	path		string	true	"Customer ID"	example(customer-001)
//	@Success		200	{object}	response.Response{data=Profile}
//	@Failure		404	{object}	response.Response
//	@Failure		429	{object}	response.Response	"Rate limit exceeded; see Retry-After header"
//	@Failure		500	{object}	response.Response
//	@Router			/api/v1/accounts/{id}/profile [get]
func (h *Handler) GetProfile(c *gin.Context) {
	customer, err := h.repo.GetByID(c.Request.Context(), c.Param("id"))
	if errors.Is(err, ErrNotFound) {
		response.NotFound(c, "Account not found", nil)
		return
	}
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "get profile failed", "error", err)
		response.InternalServerError(c, "", nil)
		return
	}

	response.Success(c, "Profile fetched successfully", ToProfile(customer))
}
