package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"goflow/backend/internal/dto"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/pkg/response"
	"goflow/backend/internal/service"
	"goflow/backend/internal/transport/http/middleware"
)

const maxUserPatchBytes = 1 << 20

// Users exposes HTTP handlers for user profile routes.
type Users struct {
	svc *service.UserService
	log *slog.Logger
}

func NewUsers(svc *service.UserService, log *slog.Logger) *Users {
	return &Users{svc: svc, log: log}
}

func (h *Users) Me(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	out, err := h.svc.Me(r.Context(), uid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Users) PatchMe(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	var body dto.PatchUserRequest
	if err := decodeUserJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.UpdateMe(r.Context(), uid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Users) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r.URL.Query().Get("limit"), 0)
	offset := queryInt(r.URL.Query().Get("offset"), 0)
	out, err := h.svc.Search(r.Context(), q, limit, offset)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Users) ByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	out, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func decodeUserJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxUserPatchBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return apperr.Validation("request body too large", err)
		}
		return apperr.Validation("invalid json body", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return apperr.Validation("request body must be a single json object", nil)
		}
		return apperr.Validation("invalid json body", err)
	}
	return nil
}

func queryInt(raw string, def int) int {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
