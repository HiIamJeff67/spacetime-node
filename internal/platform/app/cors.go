package app

import (
	"net/http"
	"strings"

	khttp "github.com/go-kratos/kratos/v3/transport/http"
)

const corsAllowedMethods = "GET,POST,PATCH,OPTIONS"
const corsAllowedHeaders = "Content-Type,Idempotency-Key,Traceparent,Tracestate"

// corsFilter applies an explicit origin allowlist to browser requests. An empty
// allowlist disables CORS instead of accidentally exposing the API with '*'.
func corsFilter(rawAllowlist string) khttp.FilterFunc {
	allowed := make(map[string]struct{})
	for _, origin := range strings.Split(rawAllowlist, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.TrimSpace(r.Header.Get("Origin"))
			_, permitted := allowed[origin]
			if origin != "" && permitted {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", corsAllowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", corsAllowedHeaders)
				w.Header().Add("Vary", "Origin")
			}
			if r.Method == http.MethodOptions && origin != "" {
				if !permitted {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
