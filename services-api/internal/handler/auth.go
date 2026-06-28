package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Architkumar13/services-catalog-api/internal/handler/middleware"
	"github.com/Architkumar13/services-catalog-api/pkg/apierror"
)

const (
	// demoUsername and demoPassword are the hardcoded credentials for the demo.
	// In production, replace with a proper user management service.
	demoUsername = "admin"
	demoPassword = "secret"
)

// tokenRequest is the JSON body expected by POST /api/v1/auth/token.
type tokenRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// tokenResponse is the JSON body returned on success.
type tokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IssueToken handles POST /api/v1/auth/token.
// It validates the demo credentials and returns a signed JWT on success.
func (h *Handlers) IssueToken(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apierror.BadRequest(w, "invalid JSON body")
		return
	}

	if req.Username != demoUsername || req.Password != demoPassword {
		apierror.Unauthorized(w, "invalid credentials")
		return
	}

	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	claims := middleware.Claims{
		Username: req.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   req.Username,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(h.jwtSecret))
	if err != nil {
		apierror.InternalServerError(w, "failed to sign token")
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{Token: signed, ExpiresAt: expiresAt})
}
