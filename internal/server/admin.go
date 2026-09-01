package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"workbuddy2api/internal/oauth"
)

const oauthFlowTTL = 10 * time.Minute

type oauthFlow struct {
	State     string
	ExpiresAt time.Time
	Polling   bool
}

func (h *Handler) apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeOpenAIError(w, http.StatusNotFound, "not_found", "API endpoint not found")
}

func (h *Handler) oauthStart(w http.ResponseWriter, r *http.Request) {
	if h.cfg.OAuth == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "oauth_unavailable", "OAuth login is unavailable")
		return
	}
	if h.cfg.Region != "cn" {
		writeOpenAIError(w, http.StatusBadRequest, "oauth_region_unsupported", "OAuth login currently supports CN accounts only")
		return
	}
	state, err := h.cfg.OAuth.Start(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "oauth_start_failed", err.Error())
		return
	}
	flowID, err := randomFlowID()
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "oauth_start_failed", "could not create OAuth flow")
		return
	}
	now := time.Now()
	h.oauthMu.Lock()
	for id, flow := range h.oauthFlows {
		if now.After(flow.ExpiresAt) {
			delete(h.oauthFlows, id)
		}
	}
	h.oauthFlows[flowID] = oauthFlow{State: state.State, ExpiresAt: now.Add(oauthFlowTTL)}
	h.oauthMu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"flow_id":    flowID,
		"auth_url":   state.AuthURL,
		"expires_in": int(oauthFlowTTL.Seconds()),
	})
}

func (h *Handler) oauthPoll(w http.ResponseWriter, r *http.Request) {
	if h.cfg.OAuth == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "oauth_unavailable", "OAuth login is unavailable")
		return
	}
	var request struct {
		FlowID string `json:"flow_id"`
	}
	if err := decodeJSONBody(r, &request); err != nil || strings.TrimSpace(request.FlowID) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request", "flow_id is required")
		return
	}
	flowID := strings.TrimSpace(request.FlowID)
	flow, status, message := h.beginOAuthPoll(flowID)
	if status != 0 {
		writeOpenAIError(w, status, "oauth_flow_invalid", message)
		return
	}
	completed := false
	defer func() { h.finishOAuthPoll(flowID, completed) }()

	account, err := h.cfg.OAuth.Poll(r.Context(), flow.State)
	if errors.Is(err, oauth.ErrPending) {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "pending"})
		return
	}
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "oauth_poll_failed", err.Error())
		return
	}
	if !validUID(account.UID) {
		writeOpenAIError(w, http.StatusBadGateway, "oauth_account_invalid", "OAuth account returned an invalid uid")
		return
	}
	if account.Region() != h.cfg.Region {
		writeOpenAIError(w, http.StatusBadRequest, "oauth_region_mismatch", "OAuth account region does not match server region")
		return
	}
	if err := os.MkdirAll(h.cfg.AuthDir, 0o700); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "auth_save_failed", err.Error())
		return
	}
	account.FilePath = filepath.Join(h.cfg.AuthDir, "workbuddy-"+account.UID+".json")
	if err := account.SaveAtomic(); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, "auth_save_failed", err.Error())
		return
	}
	h.cfg.Pool.Reauthorize(account)
	_ = h.cfg.Upstream.DailyCheckin(account)
	if credits, creditErr := h.cfg.Upstream.UserResource(account); creditErr == nil {
		h.cfg.Pool.ReenableIfCredits(account.UID, credits)
	}
	accountStatus, _ := h.cfg.Pool.Status(account.UID)
	completed = true
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "completed",
		"account": accountStatus,
	})
}

func (h *Handler) beginOAuthPoll(flowID string) (oauthFlow, int, string) {
	h.oauthMu.Lock()
	defer h.oauthMu.Unlock()
	flow, ok := h.oauthFlows[flowID]
	if !ok || time.Now().After(flow.ExpiresAt) {
		delete(h.oauthFlows, flowID)
		return oauthFlow{}, http.StatusNotFound, "OAuth flow was not found or has expired"
	}
	if flow.Polling {
		return oauthFlow{}, http.StatusConflict, "OAuth flow is already being checked"
	}
	flow.Polling = true
	h.oauthFlows[flowID] = flow
	return flow, 0, ""
}

func (h *Handler) finishOAuthPoll(flowID string, completed bool) {
	h.oauthMu.Lock()
	defer h.oauthMu.Unlock()
	if completed {
		delete(h.oauthFlows, flowID)
		return
	}
	if flow, ok := h.oauthFlows[flowID]; ok {
		flow.Polling = false
		h.oauthFlows[flowID] = flow
	}
}

func (h *Handler) checkinNow(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Maintenance == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "maintenance_unavailable", "Account maintenance is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, h.cfg.Maintenance.RunCheckinNow())
}

func (h *Handler) refreshCredits(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Maintenance == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "maintenance_unavailable", "Account maintenance is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, h.cfg.Maintenance.RefreshCreditsNow())
}

func randomFlowID() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validUID(uid string) bool {
	if len(uid) == 0 || len(uid) > 128 {
		return false
	}
	for _, char := range uid {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func decodeJSONBody(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}
