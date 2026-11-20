package matchreplace

import (
	"bytes"
	"errors"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/rawhttp"
)

// Tester caps (F-MR-1, size budget).
const (
	maxTestInput  = 12 << 20
	maxTestOutput = 16 << 20
)

// Test runs one draft rule on raw, a message of kind, and returns the output
// (raw itself when nothing changed), whether the condition matched and whether
// the output differs from raw. Only the rule's condition, section, operation,
// matcher and term are used. Every tester check of F-MR-1 is done here: err is
// an *apperr.Error (BAD_INPUT, or BAD_QUERY with span for the condition) and
// out is nil then. A rule whose result is skipped (invalid message, status not
// three digits, unsupported coding) gives raw with changed=false.
func (e *Engine) Test(r Rule, presets []filterpreset.Preset, kind Kind, raw []byte) (out []byte, matched, changed bool, err error) {
	c, err := compile(r, presets)
	if err != nil {
		return nil, false, false, err
	}
	if kind != KindRequest && kind != KindResponse {
		return nil, false, false, apperr.New(apperr.BadInput, "kind: unknown message kind %q", kind)
	}
	if c.section.response() != (kind == KindResponse) {
		return nil, false, false, apperr.New(apperr.BadInput, "section %s cannot be tested on a %s", c.section, kind)
	}
	if len(raw) > maxTestInput {
		return nil, false, false, apperr.New(apperr.BadInput, "raw must be at most 12 MiB")
	}
	x, err := testMessage(raw, kind == KindResponse)
	if err != nil {
		return nil, false, false, err
	}
	next, matched, err := step(c, x, nil, maxTestOutput)
	switch {
	case errors.Is(err, errTooLarge):
		return nil, false, false, apperr.New(apperr.BadInput, "tester output must be at most 16 MiB")
	case errors.Is(err, errSkip), errors.Is(err, errInvalid):
		e.log.Warn("matchreplace: tester rule skipped", "error", err)
		return raw, matched, false, nil
	case err != nil:
		return nil, false, false, err
	case next == nil || bytes.Equal(next.raw, raw):
		return raw, matched, false, nil
	}
	return next.raw, matched, true, nil
}

// testMessage parses tester input: one whole message, framed as rawhttp frames
// it, with no bytes after it. A response with no body bytes may declare any
// length the parser accepts (it may answer HEAD).
func testMessage(raw []byte, resp bool) (*msg, error) {
	x, err := parse(raw, resp, "")
	if err != nil && resp {
		if h, herr := parse(raw, true, "HEAD"); herr == nil && len(h.rest) == 0 {
			x, err = h, nil
		}
	}
	if err != nil {
		return nil, apperr.New(apperr.BadInput, "raw is not a valid HTTP/1.x %s: %v", kindName(resp), err)
	}
	if len(x.rest) == 0 {
		return x, nil
	}
	switch x.m.Framing() {
	case rawhttp.FramingLength:
		return nil, apperr.New(apperr.BadInput, "raw body length differs from Content-Length")
	case rawhttp.FramingNone:
		if !resp {
			return nil, apperr.New(apperr.BadInput, "raw request has bytes after the head but neither Content-Length nor Transfer-Encoding")
		}
	}
	return nil, apperr.New(apperr.BadInput, "raw has bytes after the end of the message")
}

func kindName(resp bool) string {
	if resp {
		return "response"
	}
	return "request"
}
