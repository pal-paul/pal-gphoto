package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
)

func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func respond(writer http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func writeError(writer http.ResponseWriter, status int, err error) {
	slog.Error("request failed", "status", status, "error", err)
	writeJSON(writer, status, map[string]string{"error": http.StatusText(status)})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		slog.Error("encode response", "error", err)
	}
}
