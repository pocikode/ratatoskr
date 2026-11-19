package intercept

import (
	"bufio"
	"bytes"
	"slices"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
	"github.com/pocikode/ratatoskr/internal/replay/sender"
)

// MaxEditBytes is the limit for both an edited item and its replacement.
const MaxEditBytes = 16 << 20

func replacement(kind Kind, raw []byte, updateLength bool) ([]byte, error) {
	input := bytes.NewReader(raw)
	reader := bufio.NewReader(input)
	if kind == Request {
		req, err := rawhttp.ReadRequest(reader, rawhttp.Limits{})
		if err != nil {
			return nil, apperr.New(apperr.BadInput, "invalid replacement request: %w", err)
		}
		if err := validateHead(&req.Message, req.Version()); err != nil {
			return nil, err
		}
		if updateLength {
			return sender.UpdateContentLength(raw), nil
		}

		return raw, nil
	}

	resp, err := rawhttp.ReadResponse(reader, "", rawhttp.Limits{})
	if err != nil {
		return nil, apperr.New(apperr.BadInput, "invalid replacement response: %w", err)
	}
	if err := validateHead(&resp.Message, resp.Version()); err != nil {
		return nil, err
	}
	if !updateLength {
		return raw, nil
	}

	bodyAt := len(raw) - input.Len() - reader.Buffered()

	return updateResponseLength(raw, &resp.Message, bodyAt), nil
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

func updateResponseLength(raw []byte, msg *rawhttp.Message, bodyAt int) []byte {
	if _, ok := msg.Header("Transfer-Encoding"); ok {
		return raw
	}
	body := raw[bodyAt:]
	lengths := msg.HeaderValues("Content-Length")
	if len(lengths) == 0 && len(body) == 0 || len(lengths) == 1 && lengths[0] == strconv.Itoa(len(body)) {
		return raw
	}

	var updated bytes.Buffer
	updated.Grow(len(raw))
	updated.Write(raw[:bodyAt-len(msg.Head())])
	_, _ = msg.Builder().SetBody(body).WriteTo(&updated)

	return updated.Bytes()
}
