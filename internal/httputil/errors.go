package httputil

import (
	"encoding/json"
	"log"
	"net/http"
)

// APIError представляет стандартную структуру для ответа об ошибке API.
type APIError struct {
	Code    int    `json:"code"`    // HTTP статус код ошибки.
	Message string `json:"message"` // Описание ошибки для клиента.
}

// RespondWithError отправляет JSON-ответ с ошибкой клиенту.
// Логирует ошибку на сервере (с уровнем ERROR).
func RespondWithError(w http.ResponseWriter, code int, message string) {
	// Логируем ошибку на сервере для отладки.
	log.Printf("ERROR: Responding with error: code=%d, message=%s", code, message)

	// Формируем структуру ответа.
	errResponse := APIError{
		Code:    code,
		Message: message,
	}

	// Устанавливаем заголовок Content-Type.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Устанавливаем HTTP статус код ответа.
	w.WriteHeader(code)

	// Кодируем структуру в JSON и отправляем клиенту.
	if err := json.NewEncoder(w).Encode(errResponse); err != nil {
		// Если не удалось отправить JSON, логируем и это.
		log.Printf("ERROR: Could not encode error JSON response: %v", err)
	}
}

// RespondWithJSON отправляет успешный JSON-ответ клиенту.
func RespondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		log.Printf("ERROR: Failed to marshal JSON payload: %v", err)
		RespondWithError(w, http.StatusInternalServerError, "Failed to generate response")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, err = w.Write(response)
	if err != nil {
		log.Printf("ERROR: Failed to write JSON response: %v", err)
	}
}
