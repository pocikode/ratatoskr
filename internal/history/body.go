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
	// maxDecoded bounds the decoded bytes skipped to reach the offset of one
	// call, so a decompression bomb cannot stall the server.
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

	var (
		raw        []byte
		bodyOffset int
	)

	err = db.Reader().QueryRowContext(ctx, `SELECT raw, body_offset FROM `+t+` WHERE `+k+` = ?`, id).Scan(&raw, &bodyOffset)
	if err := notFound(err, id); err != nil {
		return nil, err
	}

	return decodeWindow(raw, bodyOffset, side, offset, limit), nil
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
// window [offset, offset+limit) of it, streaming so memory is O(limit).
// Anything it cannot decode falls back to the stored bytes.
func decodeWindow(raw []byte, bodyOffset int, side Part, offset int64, limit int) []byte {
	stored := func() []byte {
		return window(raw[min(max(bodyOffset, 0), len(raw)):], offset, limit)
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

	var codings []string

	for _, v := range m.HeaderValues("Content-Encoding") {
		for c := range strings.SplitSeq(v, ",") {
			if c = strings.ToLower(strings.TrimSpace(c)); c != "" && c != "identity" {
				codings = append(codings, c)
			}
		}
	}

	r := m.DecodedBody()

	// Codings were applied in order, so they are removed in reverse.
	for _, coding := range slices.Backward(codings) {
		next, closeFn, ok := uncompress(coding, r)
		if !ok {
			// shortcut: unknown coding or unreadable header: shown as stored.
			return stored()
		}

		defer closeFn()

		r = next
	}

	// A truncated capture or stream yields what was read, so errors are dropped.
	if _, err := io.CopyN(io.Discard, r, min(offset, maxDecoded)); err != nil || offset > maxDecoded {
		return []byte{}
	}

	out, _ := io.ReadAll(io.LimitReader(r, int64(limit)))
	if len(out) == 0 && offset == 0 && len(codings) > 0 {
		return stored() // nothing decodable
	}

	return out
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

// uncompress wraps r with the decoder of one content coding.
func uncompress(coding string, r io.Reader) (io.Reader, func(), bool) {
	nop := func() {}

	switch coding {
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(r)
		if err != nil {
			return nil, nop, false
		}

		return zr, nop, true
	case "deflate":
		// HTTP deflate is zlib-wrapped, but some servers send a raw stream.
		br := bufio.NewReader(r)
		if h, _ := br.Peek(2); len(h) == 2 && h[0]&0x0f == 8 && (int(h[0])<<8|int(h[1]))%31 == 0 {
			if zr, err := zlib.NewReader(br); err == nil {
				return zr, nop, true
			}
		}

		return flate.NewReader(br), nop, true
	case "br":
		return brotli.NewReader(r), nop, true
	case "zstd":
		zr, err := zstd.NewReader(r, zstd.WithDecoderMaxMemory(maxDecoded))
		if err != nil {
			return nil, nop, false
		}

		return zr, zr.Close, true
	}

	return nil, nop, false
}
