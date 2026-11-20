package matchreplace

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// captureCap is the 64 MiB capture cap (docs/03 §4): a larger body passes
// through with no rule, and a decoded body may not exceed it either.
const captureCap = 64 << 20

// deflateRaw is a deflate body without the zlib wrapper some servers send; it
// is re-encoded the same way.
const deflateRaw = "deflate-raw"

// body applies UPDATE_RAW to the decoded body and re-frames the result.
func (c *compiled) body(x *msg, b *budget) ([]byte, error) {
	framing := x.m.Framing()
	if x.resp && framing == rawhttp.FramingNone {
		return nil, nil // HEAD, 1xx, 204, 304: an empty body that may declare any length stays
	}
	coding, err := contentCoding(x.m)
	if err != nil {
		return nil, err
	}
	payload, trailer := x.body, []byte(nil)
	if framing == rawhttp.FramingChunked {
		if payload, trailer, err = rawhttp.SplitChunkedBody(x.body); err != nil {
			return nil, err
		}
	}
	if coding == "deflate" && !isZlib(payload) {
		coding = deflateRaw
	}
	plain, err := decode(coding, payload)
	if err != nil {
		return nil, fmt.Errorf("%w: decode %s body: %w", errSkip, coding, err)
	}
	// The output cap is on wire bytes; decoded work has its own bound.
	limit := b.max(len(payload))
	if coding != "" {
		limit = captureCap
	}
	out, err := c.replace(plain, limit)
	if err != nil || bytes.Equal(out, plain) {
		return nil, err
	}
	enc, err := encode(coding, out, b.max(len(payload)))
	if err != nil {
		return nil, err
	}
	bl := x.m.Builder()
	if framing == rawhttp.FramingChunked {
		return x.with(bl.SetChunkedBody(enc, trailer).Bytes(), nil), nil
	}
	return x.with(bl.SetHeader("Content-Length", strconv.Itoa(len(enc))).Head(), enc), nil
}

// contentCoding returns the one supported Content-Encoding of m ("" for none).
// Stacked or other codings skip body rules.
func contentCoding(m *rawhttp.Message) (string, error) {
	var codings []string
	for _, v := range m.HeaderValues("Content-Encoding") {
		for t := range strings.SplitSeq(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				codings = append(codings, strings.ToLower(t))
			}
		}
	}
	switch {
	case len(codings) == 0:
		return "", nil
	case len(codings) > 1:
		return "", fmt.Errorf("%w: stacked Content-Encoding %q", errSkip, strings.Join(codings, ", "))
	}
	switch codings[0] {
	case "gzip", "deflate", "br", "zstd":
		return codings[0], nil
	}
	return "", fmt.Errorf("%w: unsupported Content-Encoding %q", errSkip, codings[0])
}

// isZlib reports whether p starts with a zlib header (RFC 1950).
func isZlib(p []byte) bool {
	return len(p) >= 2 && p[0]&0x0f == 8 && (uint16(p[0])<<8|uint16(p[1]))%31 == 0
}

func decode(coding string, p []byte) ([]byte, error) {
	src := bytes.NewReader(p)
	var r io.Reader
	switch coding {
	case "":
		return p, nil
	case "gzip":
		zr, err := gzip.NewReader(src)
		if err != nil {
			return nil, err
		}
		r = zr
	case "deflate":
		zr, err := zlib.NewReader(src)
		if err != nil {
			return nil, err
		}
		r = zr
	case deflateRaw:
		r = flate.NewReader(src)
	case "br":
		r = brotli.NewReader(src)
	case "zstd":
		d, err := zstd.NewReader(src, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(captureCap))
		if err != nil {
			return nil, err
		}
		defer d.Close()
		r = d
	}
	out, err := io.ReadAll(io.LimitReader(r, captureCap+1))
	if err != nil {
		return nil, err
	}
	if len(out) > captureCap {
		return nil, fmt.Errorf("decoded body over %d bytes", captureCap)
	}
	return out, nil
}

// capped is a buffer that fails with errTooLarge once it would exceed left.
type capped struct {
	buf  bytes.Buffer
	left int
}

func (c *capped) Write(p []byte) (int, error) {
	if len(p) > c.left {
		return 0, errTooLarge
	}
	c.left -= len(p)
	return c.buf.Write(p)
}

// encode compresses p with coding. The encoded bytes are capped at limit
// (errTooLarge) as they are written.
func encode(coding string, p []byte, limit int) ([]byte, error) {
	buf := &capped{left: limit}
	var w io.WriteCloser
	switch coding {
	case "":
		return p, nil
	case "gzip":
		w = gzip.NewWriter(buf)
	case "deflate":
		w = zlib.NewWriter(buf)
	case deflateRaw:
		fw, err := flate.NewWriter(buf, flate.DefaultCompression)
		if err != nil {
			return nil, err
		}
		w = fw
	case "br":
		w = brotli.NewWriter(buf)
	case "zstd":
		zw, err := zstd.NewWriter(buf, zstd.WithEncoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		w = zw
	}
	if _, err := w.Write(p); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.buf.Bytes(), nil
}
