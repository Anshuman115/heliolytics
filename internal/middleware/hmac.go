package middleware

import (
	"log"
	"net/http"

	"github.com/heliolytics/api/internal/auth"
)

func HMACToken(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				log.Printf("auth reject path=%s method=%s reason=server_secret_empty", r.URL.Path, r.Method)
				http.Error(w, "server misconfigured: no signing secret", http.StatusInternalServerError)
				return
			}
			tok := r.Header.Get("X-Heliolytics-Token")
			res := auth.VerifyTokenDetail(secret, tok)
			if !res.OK {
				detail := res.Detail
				if detail == "" {
					detail = "-"
				}
				log.Printf(
					"auth reject path=%s method=%s reason=%s detail=%s remote=%s token_len=%d",
					r.URL.Path, r.Method, res.Reason, detail, r.RemoteAddr, len(tok),
				)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			log.Printf("auth ok path=%s method=%s remote=%s", r.URL.Path, r.Method, r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}
}
