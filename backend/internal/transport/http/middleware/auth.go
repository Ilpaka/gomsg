package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"goflow/backend/internal/domain"
	"goflow/backend/internal/pkg/auth"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/pkg/response"
)

type userIDKey struct{}

// RequireBearerAuth validates an access JWT and stores domain.ID under this context key.
func RequireBearerAuth(secret []byte, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.WriteError(w, r, log, apperr.Unauthorized("missing bearer token"))
				return
			}
			claims, err := auth.ParseAccessToken(secret, raw)
			if err != nil {
				response.WriteError(w, r, log, apperr.Unauthorized("invalid or expired token"))
				return
			}
			uid := domain.ID(claims.UserID)
			ctx := context.WithValue(r.Context(), userIDKey{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserID returns the authenticated user id injected by RequireBearerAuth.
func UserID(ctx context.Context) (domain.ID, bool) {
	v := ctx.Value(userIDKey{})
	id, ok := v.(domain.ID)
	return id, ok
}

func bearerToken(authz string) string {
	authz = strings.TrimSpace(authz)
	if authz == "" {
		return ""
	}
	const p = "Bearer "
	if len(authz) <= len(p) || !strings.EqualFold(authz[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(authz[len(p):])
}
