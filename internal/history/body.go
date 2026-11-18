package history

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"database/sql"
	"errors"
	"io"
	"slices"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// Part is the request or the response of a stored exchange.
type Part uint8

// Parts.
const (
	PartRequest Part = iota
	PartResponse
)

// Bounds on what one body call may do.
const (
	// MaxBodyChunk is the largest limit of [Service.Body] (4 MiB).
	MaxBodyChunk = 4 << 20
	// maxDecoded bounds the decoded bytes read to serve one call, so a
	// decompression bomb cannot stall the server.
	maxDecoded = 256 << 20
)

// table and columns of each side.
func (sd Part) table() (from, key string) {
	if sd == PartResponse {
		return "responses", "request_id"
	}

	return "requests", "id"
}

// Head returns the start line and headers of a message exactly as stored.
func (s *Service) Head(ctx context.Context, id int64, side Part) ([]byte, error) {
	db, err := s.db()
	if err != nil {
		return nil, err
	}

	t, k := side.table()

	var head []byte

	err = db.Reader().QueryRowContext(ctx, `SELECT substr(raw, 1, body_offset) FROM `+t+` WHERE `+k+` = ?`, id).Scan(&head)

	return head, notFound(err, id)
}

// Body returns up to limit bytes of the stored body from offset. With decode,
// chunked framing and gzip/deflate content coding are removed first and the
// window applies to the decoded bytes (display only); a body that cannot be
// decoded is returned as stored.
func (s *Service) Body(ctx context.Context, id int64, side Part, offset int64, limit int, decode bool) ([]byte, error) {
	if offset < 0 || limit < 1 || limit > MaxBodyChunk {
		return nil, apperr.New(apperr.BadInput, "offset must be >= 0 and limit between 1 and %d", MaxBodyChunk)
	}

	db, err := s.db()
	if err != nil {
		return nil, err
	}

	t, k := side.table()

	var body []byte

	if !decode {
		err = db.Reader().QueryRowContext(ctx,
			`SELECT substr(raw, body_offset + 1 + ?, ?) FROM `+t+` WHERE `+k+` = ?`, offset, limit, id).Scan(&body)

		return nonNil(body), notFound(err, id)
	}

	var raw []byte

	err = db.Reader().QueryRowContext(ctx, `SELECT raw FROM `+t+` WHERE `+k+` = ?`, id).Scan(&raw)
	if err := notFound(err, id); err != nil {
		return nil, err
	}

	return decodeWindow(raw, side, offset, limit), nil
}

func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}

	return b
}

func notFound(err error, id int64) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return apperr.New(apperr.NotFound, "request %d not found", id)
	}

	return err
}

// decodeWindow decodes the body of the stored message raw and returns the
// window [offset, offset+limit) of it. Anything it cannot decode falls back
// to the stored bytes.
func decodeWindow(raw []byte, side Part, offset int64, limit int) []byte {
	stored := func() []byte {
		_, body, _ := bytes.Cut(raw, []byte("\r\n\r\n")) // fallback only; offsets are exact in the SQL path

		return window(body, offset, limit)
	}

	br := bufio.NewReader(bytes.NewReader(raw))

	var (
		m   *rawhttp.Message
		err error
	)

	if side == PartResponse {
		var r *rawhttp.Response

		r, err = rawhttp.ReadResponse(br, "", rawhttp.DefaultLimits)
		if r != nil {
			m = &r.Message
		}
	} else {
		var r *rawhttp.Request

		r, err = rawhttp.ReadRequest(br, rawhttp.DefaultLimits)
		if r != nil {
			m = &r.Message
		}
	}

	if err != nil || m == nil {
		return stored()
	}

	// A truncated capture yields what was stored, so the read error is dropped.
	payload, _ := io.ReadAll(io.LimitReader(m.DecodedBody(), maxDecoded))

	var codings []string

	for _, v := range m.HeaderValues("Content-Encoding") {
		for c := range strings.SplitSeq(v, ",") {
			if c = strings.ToLower(strings.TrimSpace(c)); c != "" && c != "identity" {
				codings = append(codings, c)
			}
		}
	}

	// Codings were applied in order, so they are removed in reverse.
	for _, coding := range slices.Backward(codings) {
		next, ok := uncompress(coding, payload)
		if !ok {
			// shortcut: br and zstd need github.com/andybalholm/brotli and
			// github.com/klauspost/compress (approved in docs/05, not in
			// go.mod yet); such bodies are shown as stored.
			return stored()
		}

		payload = next
	}

	return window(payload, offset, limit)
}

func window(b []byte, offset int64, limit int) []byte {
	if offset >= int64(len(b)) {
		return []byte{}
	}

	b = b[offset:]
	if len(b) > limit {
		b = b[:limit]
	}

	return bytes.Clone(b)
}

// uncompress decodes one content coding, reading at most maxDecoded bytes. A
// truncated stream yields what decoded before the cut.
func uncompress(coding string, data []byte) ([]byte, bool) {
	var r io.Reader

	switch coding {
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, false
		}

		r = zr
	case "deflate":
		// HTTP deflate is zlib-wrapped, but some servers send a raw stream.
		if zr, err := zlib.NewReader(bytes.NewReader(data)); err == nil {
			r = zr
		} else {
			r = flate.NewReader(bytes.NewReader(data))
		}
	case "br":
		r = brotli.NewReader(bytes.NewReader(data))
	case "zstd":
		zr, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderMaxMemory(maxDecoded))
		if err != nil {
			return nil, false
		}

		defer zr.Close()

		r = zr
	default:
		return nil, false
	}

	// Keep the partial output of a truncated stream.
	out, _ := io.ReadAll(io.LimitReader(r, maxDecoded))

	return out, len(out) > 0 || len(data) == 0
}
