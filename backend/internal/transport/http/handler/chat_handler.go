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

const maxChatBodyBytes = 1 << 20

// Chats exposes HTTP handlers for chat routes.
type Chats struct {
	svc *service.ChatService
	log *slog.Logger
}

func NewChats(svc *service.ChatService, log *slog.Logger) *Chats {
	return &Chats{svc: svc, log: log}
}

func (h *Chats) CreateDirect(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	var body dto.CreateDirectChatRequest
	if err := decodeChatJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.CreateDirect(r.Context(), uid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, out)
}

func (h *Chats) CreateGroup(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	var body dto.CreateGroupChatRequest
	if err := decodeChatJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.CreateGroup(r.Context(), uid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusCreated, out)
}

func (h *Chats) List(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	limit := queryIntChats(r.URL.Query().Get("limit"), 0)
	offset := queryIntChats(r.URL.Query().Get("offset"), 0)
	out, err := h.svc.ListMine(r.Context(), uid, limit, offset)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Chats) Get(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	out, err := h.svc.Get(r.Context(), uid, cid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Chats) Members(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	out, err := h.svc.Members(r.Context(), uid, cid)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Chats) AddMembers(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	var body dto.AddChatMembersRequest
	if err := decodeChatJSON(w, r, &body); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	out, err := h.svc.AddMembers(r.Context(), uid, cid, body)
	if err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, out)
}

func (h *Chats) RemoveMember(w http.ResponseWriter, r *http.Request) {
	uid, _ := middleware.UserID(r.Context())
	cid := strings.TrimSpace(r.PathValue("chat_id"))
	target := strings.TrimSpace(r.PathValue("user_id"))
	if err := h.svc.RemoveMember(r.Context(), uid, cid, target); err != nil {
		response.WriteError(w, r, h.log, err)
		return
	}
	response.WriteSuccess(w, http.StatusOK, map[string]any{"removed": true})
}

func decodeChatJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxChatBodyBytes)
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

func queryIntChats(raw string, def int) int {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}
