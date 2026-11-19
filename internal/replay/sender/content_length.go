// Package sender sends raw HTTP/1.x requests over a fresh connection and reads the response with rawhttp.
package sender

import (
	"bufio"
	"bytes"
	"slices"
	"strconv"

	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// UpdateContentLength sets Content-Length to the size of the editor body.
// Requests with Transfer-Encoding or heads rejected by rawhttp stay unchanged.
func UpdateContentLength(raw []byte) []byte {
	input := bytes.NewReader(raw)
	reader := bufio.NewReader(input)
	req, err := rawhttp.ReadRequest(reader, rawhttp.Limits{})
	if err != nil || slices.Contains(req.Warnings(), rawhttp.WarnMalformedHeader) {
		return raw
	}

	bodyAt := len(raw) - input.Len() - reader.Buffered()

	return UpdateParsedContentLength(raw, &req.Message, bodyAt)
}

// UpdateParsedContentLength applies the editor length rule to a parsed head.
// bodyAt is the offset just after the head, including any leading blank lines.
func UpdateParsedContentLength(raw []byte, msg *rawhttp.Message, bodyAt int) []byte {
	if _, ok := msg.Header("Transfer-Encoding"); ok {
		return raw
	}
	// The editor body is everything after the head, regardless of the old length.
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
