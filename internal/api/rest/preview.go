package rest

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/history"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

const previewPrefix = "/api/rest/preview/"

// ResponseHistory is the stored history read by the response preview route.
type ResponseHistory interface {
	Head(ctx context.Context, id int64, side history.Part) ([]byte, error)
	Body(ctx context.Context, id int64, side history.Part, offset int64, limit int, decode bool) ([]byte, error)
}

// WithPreviewHeaders sets the sandbox policy even when middleware rejects a preview request.
func WithPreviewHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, previewPrefix) {
			h := w.Header()
			h.Set("Content-Security-Policy", "sandbox")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Cache-Control", "no-store")
			h.Set("Referrer-Policy", "no-referrer")
		}
		next.ServeHTTP(w, r)
	})
}

func servePreview(source ResponseHistory) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)

			return
		}

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			http.NotFound(w, r)

			return
		}

		head, err := source.Head(r.Context(), id, history.PartResponse)
		if err != nil {
			writePreviewError(w, r, id, err)

			return
		}

		resp, err := rawhttp.ReadResponse(bufio.NewReader(bytes.NewReader(head)), "", rawhttp.DefaultLimits)
		if err != nil {
			writePreviewError(w, r, id, err)

			return
		}

		body, err := source.Body(r.Context(), id, history.PartResponse, 0, history.MaxBodyChunk, true)
		if err != nil {
			writePreviewError(w, r, id, err)

			return
		}

		contentType, ok := resp.Header("Content-Type")
		if !ok {
			contentType = "application/octet-stream"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodGet {
			_, _ = w.Write(body)
		}
	}
}

func writePreviewError(w http.ResponseWriter, r *http.Request, id int64, err error) {
	switch apperr.CodeOf(err) {
	case apperr.NotFound, apperr.Conflict:
		http.NotFound(w, r)
	default:
		slog.WarnContext(r.Context(), "response preview failed", "request_id", id, "error", err)
		http.Error(w, "response preview unavailable", http.StatusInternalServerError)
	}
}
