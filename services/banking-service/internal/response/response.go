// Package response writes every API reply in one standard envelope:
//
//	{"status": 200, "message": "...", "success": true, "data": ...}
//
// Each helper takes an optional message; pass "" to use the default.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the standard envelope for all API responses.
type Response struct {
	Status  int    `json:"status" example:"200"`
	Message string `json:"message" example:"Request successful"`
	Success bool   `json:"success" example:"true"`
	Data    any    `json:"data"`
}

var validStatusCodes = map[int]bool{
	http.StatusOK:                  true,
	http.StatusCreated:             true,
	http.StatusBadRequest:          true,
	http.StatusUnauthorized:        true,
	http.StatusForbidden:           true,
	http.StatusNotFound:            true,
	http.StatusMethodNotAllowed:    true,
	http.StatusConflict:            true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusServiceUnavailable:  true,
}

// Create writes the envelope. Unsupported status codes become a 500.
func Create(c *gin.Context, status int, message string, success bool, data any) {
	if !validStatusCodes[status] {
		status = http.StatusInternalServerError
		message = "Invalid status code, defaulting to 500 Internal Server Error"
		success = false
	}

	c.JSON(status, Response{
		Status:  status,
		Message: message,
		Success: success,
		Data:    data,
	})
}

func orDefault(message, fallback string) string {
	if message == "" {
		return fallback
	}
	return message
}

// Success writes 200 OK.
func Success(c *gin.Context, message string, data any) {
	Create(c, http.StatusOK, orDefault(message, "Success"), true, data)
}

// Created writes 201 Created.
func Created(c *gin.Context, message string, data any) {
	Create(c, http.StatusCreated, orDefault(message, "Resource created successfully"), true, data)
}

// BadRequest writes 400 Bad Request.
func BadRequest(c *gin.Context, message string, data any) {
	Create(c, http.StatusBadRequest, orDefault(message, "Bad request from client"), false, data)
}

// Unauthorized writes 401 Unauthorized.
func Unauthorized(c *gin.Context, message string, data any) {
	Create(c, http.StatusUnauthorized, orDefault(message, "Invalid credentials provided"), false, data)
}

// Forbidden writes 403 Forbidden.
func Forbidden(c *gin.Context, message string, data any) {
	Create(c, http.StatusForbidden, orDefault(message, "Access is forbidden to this resource"), false, data)
}

// NotFound writes 404 Not Found.
func NotFound(c *gin.Context, message string, data any) {
	Create(c, http.StatusNotFound, orDefault(message, "Resource not found"), false, data)
}

// MethodNotAllowed writes 405 Method Not Allowed.
func MethodNotAllowed(c *gin.Context, message string, data any) {
	Create(c, http.StatusMethodNotAllowed, orDefault(message, "Method not allowed for this route"), false, data)
}

// Conflict writes 409 Conflict.
func Conflict(c *gin.Context, message string, data any) {
	Create(c, http.StatusConflict, orDefault(message, "Conflict in resource"), false, data)
}

// TooManyRequests writes 429 Too Many Requests.
func TooManyRequests(c *gin.Context, message string, data any) {
	Create(c, http.StatusTooManyRequests, orDefault(message, "Too many requests, please retry later"), false, data)
}

// InternalServerError writes 500 Internal Server Error.
func InternalServerError(c *gin.Context, message string, data any) {
	Create(c, http.StatusInternalServerError, orDefault(message, "An unexpected error occurred on the server"), false, data)
}

// ServiceUnavailable writes 503 Service Unavailable, used by the health check.
func ServiceUnavailable(c *gin.Context, message string, data any) {
	Create(c, http.StatusServiceUnavailable, orDefault(message, "Service unavailable"), false, data)
}
