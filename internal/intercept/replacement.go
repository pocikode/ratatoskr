package intercept

import (
	"bufio"
	"bytes"
	"slices"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/replay/sender"
)

// MaxEditBytes is the limit for both an edited item and its replacement.
const MaxEditBytes = 16 << 20

func replacement(kind Kind, method string, raw []byte, updateLength bool) ([]byte, error) {
	input := bytes.NewReader(raw)
	reader := bufio.NewReader(input)
	var msg *rawhttp.Message
	var version string
	if kind == Request {
		req, err := rawhttp.ReadRequest(reader, rawhttp.Limits{})
		if err != nil {
			return nil, apperr.New(apperr.BadInput, "invalid replacement request: %w", err)
		}
		msg, version = &req.Message, req.Version()
	} else {
		resp, err := rawhttp.ReadResponse(reader, method, rawhttp.Limits{})
		if err != nil {
			return nil, apperr.New(apperr.BadInput, "invalid replacement response: %w", err)
		}
		msg, version = &resp.Message, resp.Version()
		updateLength = updateLength && resp.Framing() != rawhttp.FramingNone
	}
	if err := validateHead(msg, version); err != nil {
		return nil, err
	}
	if !updateLength {
		return raw, nil
	}

	bodyAt := len(raw) - input.Len() - reader.Buffered()

	return sender.UpdateParsedContentLength(raw, msg, bodyAt), nil
}

func validateHead(msg *rawhttp.Message, version string) error {
	if version != "HTTP/1.0" && version != "HTTP/1.1" {
		return apperr.New(apperr.BadInput, "invalid replacement HTTP version %q", version)
	}
	if slices.Contains(msg.Warnings(), rawhttp.WarnMalformedHeader) {
		return apperr.New(apperr.BadInput, "invalid replacement: %w: malformed header", rawhttp.ErrMalformed)
	}

	return nil
}
