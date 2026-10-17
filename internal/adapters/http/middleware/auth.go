package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/diogo/golangbet/internal/adapters/auth"
)

func Authenticate(v *auth.Validator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := v.Authenticate(r.Context(), r)
			if err != nil {
				writeErr(w, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
		})
	}
}

func RequireInternal(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.FromContext(r.Context())
		if !ok || !p.Internal {
			writeErr(w, http.StatusForbidden, "FORBIDDEN", "internal role required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}
