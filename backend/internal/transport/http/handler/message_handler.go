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

const maxMessageBodyBytes = 1 << 20

// Messages exposes HTTP handlers for chat messages and read receipts.
type Messages struct {
	svc *service.MessageService
	log *slog.Logger
}

func NewMessages(svc *service.MessageService, log *slog.Logger) *Messages {
	return &Messages{svc: svc, log: log}
}

func (h *Messages) ListByChat(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	limit := queryIntMessages(r.URL.Query().Get("limit"), 0)
	before := strings.TrimSpace(r.URL.Query().Get("before"))
	out, err := h.svc.ListByChat(r.Context(), uid, cid, limit, before)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Messages) CreateInChat(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	var body dto.CreateMessageRequest
	if err := decodeMessageJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.Create(r.Context(), uid, cid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, out)
}

func (h *Messages) Get(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	mid := strings.TrimSpace(r.PathValue("message_id"))
	out, err := h.svc.Get(r.Context(), uid, mid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Messages) Patch(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	mid := strings.TrimSpace(r.PathValue("message_id"))
	var body dto.PatchMessageRequest
	if err := decodeMessageJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.Patch(r.Context(), uid, mid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Messages) Delete(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	mid := strings.TrimSpace(r.PathValue("message_id"))
	if err := h.svc.Delete(r.Context(), uid, mid); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, map[string]any{"deleted": true})
}

func (h *Messages) MarkRead(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	mid := strings.TrimSpace(r.PathValue("message_id"))
	out, err := h.svc.MarkRead(r.Context(), uid, mid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func queryIntMessages(raw string, def int) int {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func decodeMessageJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxMessageBodyBytes)
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
