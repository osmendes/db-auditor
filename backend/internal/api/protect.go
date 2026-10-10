package api

import (
	"net/http"
	"strings"
)

const maxAPIBody = 1 << 20

func protectPublic(next http.Handler) http.Handler {
	mutations := &apiWindow{byKey: make(map[string]attemptWindow), max: 20}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			writeError(w, http.StatusNotFound, CodeNotFound, "Métricas não são publicadas neste endpoint.")
			return
		}
		if r.ContentLength > maxAPIBody {
			writeError(w, http.StatusRequestEntityTooLarge, CodeValidation, "O corpo da requisição passa de 1 MiB.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxAPIBody)
		if r.Method == http.MethodPost && mutationPath(r.URL.Path) && !mutations.allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, CodeUnavailable, "Muitas requisições. Aguarde um minuto.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func mutationPath(path string) bool {
	return strings.Contains(path, "/audit-runs") || strings.Contains(path, "/reports")
}
