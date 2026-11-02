package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hyperheaven04/go_chat/backend/internal/domain"
	"github.com/hyperheaven04/go_chat/backend/internal/transport/dto"
)

const maxBodyBytes = 64 << 10

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *API) writeError(w http.ResponseWriter, r *http.Request, err error) {
	e := dto.ClassifyError(err)
	if e.Status >= 500 {
		a.Log.ErrorContext(r.Context(), "request failed", "error", err, "request_id", requestIDFrom(r.Context()))
	}
	writeJSON(w, e.Status, map[string]dto.Error{"error": e})
}

// decodeJSON reads exactly one JSON object with known fields from a size-limited body.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return domain.NewValidationError("body", "request body too large")
		}
		return domain.NewValidationError("body", "invalid JSON")
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		return domain.NewValidationError("body", "must contain a single JSON object")
	}
	return nil
}
