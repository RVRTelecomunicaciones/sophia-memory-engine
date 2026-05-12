package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/sophia-engine/memory-engine/internal/domain/auth"
	"github.com/sophia-engine/memory-engine/internal/ports/inbound"
)

// APIKey returns a middleware that enforces API-key authentication.
//
// The middleware reads the X-API-Key request header, delegates validation to svc,
// and on success injects the resulting AuthContext into the request context.
//
// All authentication failures (missing header, wrong key, revoked key, too short,
// repo error) produce an identical 401 JSON body with the generic message
// "unauthorized". This prevents callers from distinguishing between failure modes.
//
// SECURITY NOTE: The X-API-Key header value is NEVER logged.
func APIKey(svc inbound.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				writeUnauthorized(w, "missing X-API-Key header")
				return
			}

			ac, err := svc.Authenticate(r.Context(), key)
			if err != nil {
				// Do NOT echo the specific error reason — same 401 for all failure modes.
				writeUnauthorized(w, "unauthorized")
				return
			}

			ctx := auth.NewContext(r.Context(), ac)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeUnauthorized writes a 401 JSON response. The message must never contain
// information that distinguishes one failure mode from another.
func writeUnauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	json.NewEncoder(w).Encode(map[string]string{"error": msg}) //nolint:errcheck
}
