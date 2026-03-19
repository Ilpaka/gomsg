package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/pkg/response"
	"goflow/backend/internal/service"
	"goflow/backend/internal/transport/http/middleware"
)

const maxAuthBodyBytes = 1 << 20

// Auth exposes HTTP handlers for authentication routes.
type Auth struct {
	svc *service.AuthService
	log *slog.Logger
}

func NewAuth(svc *service.AuthService, log *slog.Logger) *Auth {
	return &Auth{svc: svc, log: log}
}

func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var body dto.RegisterRequest
	if err := decodeJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.Register(r.Context(), body, clientMeta(r))
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, out)
}

func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var body dto.LoginRequest
	if err := decodeJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.Login(r.Context(), body, clientMeta(r))
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	var body dto.RefreshRequest
	if err := decodeJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.Refresh(r.Context(), body.RefreshToken)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	var body dto.LogoutRequest
	if err := decodeJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	if err := h.svc.Logout(r.Context(), body.RefreshToken); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (h *Auth) LogoutAll(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserID(r.Context())
	if !ok {
		response.WriteError(w, r, h.log, apperr.Unauthorized("missing user context"))
		return
	}
	if err := h.svc.LogoutAll(r.Context(), uid); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, map[string]any{"logged_out_all": true})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperr.Validation("request body too large", err)
		}
		return apperr.Validation("invalid json body", err)
	}
	return nil
}

func clientMeta(r *http.Request) service.ClientMeta {
	return service.ClientMeta{
		UserAgent: r.UserAgent(),
		IP:        r.RemoteAddr,
	}
}
