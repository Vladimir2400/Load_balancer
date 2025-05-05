package adminapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"cloud/load_balancer/internal/httputil"
	rl "cloud/load_balancer/internal/ratelimiter"
)

// Структура для запроса на создание/обновление лимита
type setLimitRequest struct {
	ClientID string  `json:"client_id"`
	Capacity int64   `json:"capacity"`
	Rate     float64 `json:"rate"`
}

// Структура для ответа с информацией о лимите
type limitResponse struct {
	ClientID string  `json:"client_id"`
	Capacity int64   `json:"capacity"`
	Rate     float64 `json:"rate"`
}

// AdminHandler обрабатывает запросы к Admin API.
type AdminHandler struct {
	manager rl.LimitManager
}

// NewAdminHandler создает новый обработчик Admin API.
func NewAdminHandler(m rl.LimitManager) *AdminHandler {
	if m == nil {
		panic("LimitManager cannot be nil for AdminHandler")
	}
	return &AdminHandler{manager: m}
}

// ServeHTTP основной маршрутизатор для /admin/limits
func (h *AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/admin/limits")
	// Убираем слеши по краям
	path = strings.Trim(path, "/")

	switch r.Method {
	case http.MethodPost:
		if path == "" {
			h.handleSetLimit(w, r)
		} else {
			httputil.RespondWithError(w, http.StatusMethodNotAllowed, "Method Not Allowed (POST expects no client ID in path)")
		}
	case http.MethodGet:
		if path != "" {
			h.handleGetLimit(w, r, path)
		} else {
			// GET /admin/limits
			httputil.RespondWithError(w, http.StatusNotImplemented, "Listing limits is not implemented")
		}
	case http.MethodDelete:
		// DELETE /admin/limits/{client_id} - Удаление лимита
		if path != "" {
			h.handleDeleteLimit(w, r, path) // path здесь - это client_id
		} else {
			httputil.RespondWithError(w, http.StatusMethodNotAllowed, "Method Not Allowed (DELETE expects client ID in path)")
		}
	default:
		httputil.RespondWithError(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

// handleSetLimit обрабатывает POST /admin/limits
func (h *AdminHandler) handleSetLimit(w http.ResponseWriter, r *http.Request) {
	var req setLimitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.RespondWithError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	defer r.Body.Close()

	if req.ClientID == "" {
		httputil.RespondWithError(w, http.StatusBadRequest, "client_id is required")
		return
	}
	if req.Capacity <= 0 {
		httputil.RespondWithError(w, http.StatusBadRequest, "capacity must be positive")
		return
	}
	if req.Rate <= 0 {
		httputil.RespondWithError(w, http.StatusBadRequest, "rate must be positive")
		return
	}

	err := h.manager.SetLimit(req.ClientID, req.Capacity, req.Rate)
	if err != nil {
		httputil.RespondWithError(w, http.StatusInternalServerError, "Failed to set limit: "+err.Error())
		return
	}

	resp := limitResponse{
		ClientID: req.ClientID,
		Capacity: req.Capacity,
		Rate:     req.Rate,
	}
	httputil.RespondWithJSON(w, http.StatusOK, resp)
}

// handleGetLimit обрабатывает GET /admin/limits/{client_id}
func (h *AdminHandler) handleGetLimit(w http.ResponseWriter, r *http.Request, clientID string) {
	if clientID == "" { // Дополнительная проверка
		httputil.RespondWithError(w, http.StatusBadRequest, "Client ID missing in path")
		return
	}

	capacity, rate, found := h.manager.GetLimit(clientID)
	if !found {
		httputil.RespondWithError(w, http.StatusNotFound, "Limit not found for client "+clientID)
		return
	}

	resp := limitResponse{
		ClientID: clientID,
		Capacity: capacity,
		Rate:     rate,
	}
	httputil.RespondWithJSON(w, http.StatusOK, resp)
}

// handleDeleteLimit обрабатывает DELETE /admin/limits/{client_id}
func (h *AdminHandler) handleDeleteLimit(w http.ResponseWriter, r *http.Request, clientID string) {
	if clientID == "" {
		httputil.RespondWithError(w, http.StatusBadRequest, "Client ID missing in path")
		return
	}

	err := h.manager.DeleteLimit(clientID)
	if err != nil {
		httputil.RespondWithError(w, http.StatusInternalServerError, "Failed to delete limit: "+err.Error())
		return
	}

	// Успешное удаление (или лимит не был найден)
	w.WriteHeader(http.StatusNoContent)
}
