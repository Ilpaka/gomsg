package handler

import (
	"log/slog"
	"net/http"

	"goflow/backend/internal/pkg/response"
	apperr "goflow/backend/internal/pkg/errors"
	"goflow/backend/internal/service"
	"goflow/backend/internal/transport/http/middleware"
)

// WSTicket exposes POST /ws/ticket for short-lived connect tickets.
type WSTicket struct {
	svc *service.WSTicketService
	log *slog.Logger
}

func NewWSTicket(svc *service.WSTicketService, log *slog.Logger) *WSTicket {
	return &WSTicket{svc: svc, log: log}
}

func (h *WSTicket) Issue(w http.ResponseWriter, r *http.Request) {
	uid, ok := middleware.UserID(r.Context())
	if !ok {
		response.WriteError(w, r, h.log, apperr.Unauthorized("missing user context"))
		return
	}
	tok, sec, err := h.svc.Issue(r.Context(), uid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, map[string]any{
		"ticket":     tok,
		"expires_in": sec,
	})
}
