package apierror

import (
	"encoding/json"
	"net/http"
)

// Response is the standard JSON error body returned by the API.
type Response struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Write serialises an error response and sets the appropriate HTTP status.
func Write(w http.ResponseWriter, status int, msg, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response{Error: msg, Code: code})
}

// Common helpers.

func BadRequest(w http.ResponseWriter, msg string) {
	Write(w, http.StatusBadRequest, msg, "BAD_REQUEST")
}

func Unauthorized(w http.ResponseWriter, msg string) {
	Write(w, http.StatusUnauthorized, msg, "UNAUTHORIZED")
}

func Forbidden(w http.ResponseWriter, msg string) {
	Write(w, http.StatusForbidden, msg, "FORBIDDEN")
}

func NotFound(w http.ResponseWriter, msg string) {
	Write(w, http.StatusNotFound, msg, "NOT_FOUND")
}

func Conflict(w http.ResponseWriter, msg string) {
	Write(w, http.StatusConflict, msg, "CONFLICT")
}

func InternalServerError(w http.ResponseWriter, msg string) {
	Write(w, http.StatusInternalServerError, msg, "INTERNAL_ERROR")
}

func UnprocessableEntity(w http.ResponseWriter, msg string) {
	Write(w, http.StatusUnprocessableEntity, msg, "UNPROCESSABLE_ENTITY")
}
