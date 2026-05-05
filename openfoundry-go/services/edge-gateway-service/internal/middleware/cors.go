package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/cors"
)

// CORSAllowedMethods are the methods the gateway forwards.
var CORSAllowedMethods = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"}

// CORS returns a chi-compatible CORS middleware.
//
// When `origins` is empty the gateway allows any origin (anonymous
// browse) — matches the Rust default `Any`. When non-empty, only the
// listed origins are accepted and credentials are allowed.
func CORS(origins []string) func(next http.Handler) http.Handler {
	c := cors.Options{
		AllowedMethods: CORSAllowedMethods,
		AllowedHeaders: []string{"*"},
		MaxAge:         int((1 * time.Hour) / time.Second),
	}
	if len(origins) == 0 {
		c.AllowedOrigins = []string{"*"}
	} else {
		c.AllowedOrigins = append([]string(nil), origins...)
		c.AllowCredentials = true
	}
	return cors.Handler(c)
}
