package api

import (
	"net/http"
	"sentinel/logger"
	"strings"
	"time"
)

// MiddlewareFunc is a function that wraps an http.HandlerFunc
type MiddlewareFunc func(http.HandlerFunc) http.HandlerFunc

// Chain applies middlewares to a handler
func Chain(handler http.HandlerFunc, middlewares ...MiddlewareFunc) http.HandlerFunc {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// AuthMiddleware checks API token
func AuthMiddleware(token string) MiddlewareFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if no token configured
			if token == "" {
				next(w, r)
				return
			}

			// Get token from header
			authHeader := r.Header.Get("Authorization")

			// Check Bearer token
			if !strings.HasPrefix(authHeader, "Bearer ") {
				writeJSON(w, http.StatusUnauthorized, Response{
					Success:   false,
					Message:   "Unauthorized - Bearer token required",
					Timestamp: time.Now(),
				})
				return
			}

			// Validate token
			requestToken := strings.TrimPrefix(authHeader, "Bearer ")
			if requestToken != token {
				logger.Log.Warnf("Invalid API token from %s", r.RemoteAddr)
				writeJSON(w, http.StatusUnauthorized, Response{
					Success:   false,
					Message:   "Unauthorized - Invalid token",
					Timestamp: time.Now(),
				})
				return
			}

			// Token valid - continue
			next(w, r)
		}
	}
}

// LoggerMiddleware logs all requests
func LoggerMiddleware() MiddlewareFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Process request
			next(w, r)

			// Log request
			logger.Log.Infof("API %s %s %s",
				r.Method,
				r.URL.Path,
				time.Since(start),
			)
		}
	}
}