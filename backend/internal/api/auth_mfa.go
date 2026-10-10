package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/mayconmendes-qc/db-auditor/internal/repository"
)

type totpGate interface {
	TOTPState(context.Context, string) (repository.TOTPState, error)
	BeginTOTPEnrollment(context.Context, string, string) (string, string, []string, error)
	ConfirmTOTP(context.Context, string, string, time.Time) error
	SetTOTPRequired(context.Context, string, bool) error
	DisableTOTP(context.Context, string) error
	CreateMFAChallenge(context.Context, string, []byte, time.Time) error
	ConsumeMFAChallenge(context.Context, []byte, string, time.Time) (string, error)
	FindAuditorUserByID(context.Context, string) (*repository.AuditorUser, error)
}

func registerTOTPRoutes(mux *http.ServeMux, store AuthStore) {
	gate, ok := store.(totpGate)
	if !ok {
		return
	}
	mux.HandleFunc("POST /api/v1/auth/totp", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var body struct {
			MFAToken string `json:"mfa_token"`
			Code     string `json:"code"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.MFAToken == "" || body.Code == "" {
			writeError(w, http.StatusBadRequest, CodeValidation, "Código inválido.")
			return
		}
		userID, err := gate.ConsumeMFAChallenge(r.Context(), repository.MFATokenHash(body.MFAToken), body.Code, time.Now())
		if err != nil {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Código inválido.")
			return
		}
		user, err := gate.FindAuditorUserByID(r.Context(), userID)
		if err != nil || user == nil || !user.Active {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Código inválido.")
			return
		}
		writeSession(w, r, store, user)
	})
	mux.HandleFunc("POST /api/v1/auth/totp/enroll", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := requestIdentity(r)
		if user == nil {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Sessão ausente.")
			return
		}
		secret, uri, codes, err := gate.BeginTOTPEnrollment(r.Context(), user.ID, user.Username)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, CodeUnavailable, "Não foi possível iniciar o TOTP. Confira AUDITOR_TOTP_KEY.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "otpauth_uri": uri, "recovery_codes": codes})
	})
	mux.HandleFunc("POST /api/v1/auth/totp/confirm", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := requestIdentity(r)
		var body struct{ Code string }
		if user == nil || json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Código inválido.")
			return
		}
		if err := gate.ConfirmTOTP(r.Context(), user.ID, strings.TrimSpace(body.Code), time.Now()); err != nil {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Código inválido.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"totp_enabled": true})
	})
	mux.HandleFunc("POST /api/v1/auth/totp/require", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := requestIdentity(r)
		var body struct{ Required bool }
		if user == nil || user.Role != "operator" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 256)).Decode(&body) != nil {
			writeError(w, http.StatusForbidden, CodeUnavailable, "Somente um operador pode exigir TOTP na própria conta.")
			return
		}
		if err := gate.SetTOTPRequired(r.Context(), user.ID, body.Required); err != nil {
			writeError(w, http.StatusConflict, CodeConflict, "Ative o TOTP antes de exigí-lo.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"totp_required": body.Required})
	})
	mux.HandleFunc("POST /api/v1/auth/totp/disable", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user := requestIdentity(r)
		var body struct{ Password, Code string }
		if user == nil || json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body) != nil {
			writeError(w, http.StatusBadRequest, CodeValidation, "Confirme a senha e o código.")
			return
		}
		full, err := gate.FindAuditorUserByID(r.Context(), user.ID)
		if err != nil || full == nil || !repository.CheckAuditorPassword(body.Password, full.PasswordHash) {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Credenciais inválidas.")
			return
		}
		state, err := gate.TOTPState(r.Context(), user.ID)
		if err != nil || (state.Enabled && !repository.CheckAuditorPassword(body.Password, full.PasswordHash)) {
			writeError(w, http.StatusUnauthorized, CodeUnavailable, "Credenciais inválidas.")
			return
		}
		if state.Enabled {
			if err := gate.ConfirmTOTP(r.Context(), user.ID, strings.TrimSpace(body.Code), time.Now()); err != nil {
				writeError(w, http.StatusUnauthorized, CodeUnavailable, "Código inválido.")
				return
			}
		}
		if err := gate.DisableTOTP(r.Context(), user.ID); err != nil {
			writeError(w, http.StatusInternalServerError, CodeInternal, "Não foi possível desativar o TOTP.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"totp_enabled": false})
	})
}

// respondTOTPChallenge writes the MFA step and reports whether login must stop.
// A store without TOTP keeps the password session unchanged.
func respondTOTPChallenge(w http.ResponseWriter, r *http.Request, store AuthStore, user *repository.AuditorUser) bool {
	gate, ok := store.(totpGate)
	if !ok || user == nil {
		return false
	}
	state, err := gate.TOTPState(r.Context(), user.ID)
	if err != nil || !state.Enabled {
		return false
	}
	challenge := make([]byte, 32)
	if _, err = rand.Read(challenge); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao criar sessão.")
		return true
	}
	encoded := base64.RawURLEncoding.EncodeToString(challenge)
	sum := sha256.Sum256([]byte(encoded))
	if err = gate.CreateMFAChallenge(r.Context(), user.ID, sum[:], time.Now().Add(5*time.Minute)); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao criar sessão.")
		return true
	}
	writeJSON(w, http.StatusOK, map[string]any{"mfa_required": true, "mfa_token": encoded, "expires_in": 300})
	return true
}

func writeSession(w http.ResponseWriter, r *http.Request, store AuthStore, user *repository.AuditorUser) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao criar sessão.")
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(token)
	digest := sha256.Sum256([]byte(encoded))
	if err := store.CreateAuditorSession(r.Context(), user.ID, digest[:], time.Now().Add(8*time.Hour)); err != nil {
		writeError(w, http.StatusInternalServerError, CodeInternal, "Falha ao criar sessão.")
		return
	}
	if r.URL.Query().Get("mode") == "cookie" {
		http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: encoded, Path: "/", MaxAge: 28800,
			HttpOnly: true, Secure: secureRequest(r), SameSite: http.SameSiteLaxMode})
		writeJSON(w, http.StatusOK, map[string]any{"user": user, "csrf_token": csrfToken(encoded), "expires_in": 28800})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": encoded, "user": user, "expires_in": 28800})
}
