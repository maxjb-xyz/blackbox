package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// decodeJSONBody decodes a JSON request body with size limits and proper error handling.
// Returns true on success, false on failure after writing the response.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, maxBytes int64, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(v)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}

	var extra interface{}
	err = decoder.Decode(&extra)
	if err != io.EOF {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		writeError(w, http.StatusBadRequest, "request body must only contain a single JSON value")
		return false
	}

	return true
}
