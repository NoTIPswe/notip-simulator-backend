package http

import (
	"crypto/subtle"
	"net/http"
)

func SimTokenMiddleware(expectedToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if expectedToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("X-Sim-Token")
		if token == "" {
			http.Error(w, "Unauthorized: missing X-Sim-Token header", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
			http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
