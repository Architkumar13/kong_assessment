package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/Architkumar13/services-catalog-api/pkg/apierror"
)

// claimsKey is the context key used to store validated JWT claims.
type claimsKey struct{}

// Claims holds the JWT payload that is injected into request contexts.
type Claims struct {
	Username string
	jwt.RegisteredClaims
}

// ClaimsFromContext retrieves the JWT claims stored by the Auth middleware.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*Claims)
	return c, ok
}

// Auth returns a middleware that validates a Bearer JWT in the Authorization header.
// It uses the provided secret for HMAC-SHA256 verification.
func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				apierror.Unauthorized(w, "missing Authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				apierror.Unauthorized(w, "Authorization header must use Bearer scheme")
				return
			}

			tokenStr := parts[1]
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				apierror.Unauthorized(w, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey{}, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
