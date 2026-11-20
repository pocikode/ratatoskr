package matchreplace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/pocikode/ratatoskr/internal/filterpreset"
)

// proxyCap bounds the result of one rule on traffic; a larger result skips the
// rule. shortcut: F-MR-1 sets no proxy output cap, this only bounds memory;
// revisit if real traffic needs more than 256 MiB.
const proxyCap = 256 << 20

// Engine applies the enabled rules to traffic. It satisfies the proxy's
// Rewriter interface (T-105) and is safe for concurrent use; [Engine.Set]
// swaps the rule list atomically.
type Engine struct {
	log   *slog.Logger
	rules atomic.Pointer[[]*compiled]
}

// New returns an engine with no rules.
func New(log *slog.Logger) *Engine {
	return &Engine{log: log.With("component", "matchreplace")}
}

// Set replaces the rules: enabled rules, in order (collections in order, then
// rules in order). Each condition is expanded against presets. A rule that no
// longer compiles (deleted preset, unavailable field) is skipped and logged.
func (e *Engine) Set(rules []Rule, presets []filterpreset.Preset) {
	list := make([]*compiled, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		c, err := compile(r, presets)
		if err != nil {
			e.log.Warn("matchreplace: rule skipped", "rule", r.Name, "error", err)
			continue
		}
		list = append(list, c)
	}
	e.rules.Store(&list)
}

// Active reports whether an enabled rule exists for source and side.
func (e *Engine) Active(source string, response bool) bool {
	return len(e.active(source, response)) > 0
}

func (e *Engine) active(source string, response bool) []*compiled {
	p := e.rules.Load()
	if p == nil {
		return nil
	}
	var out []*compiled
	for _, c := range *p {
		if c.applies(source, response) {
			out = append(out, c)
		}
	}
	return out
}

// Request returns the rewritten request, or raw itself when nothing changed.
// The error is non-nil only when raw does not parse or ctx is done; the caller
// then keeps raw.
func (e *Engine) Request(ctx context.Context, source string, raw []byte) ([]byte, error) {
	rules := e.active(source, false)
	if len(rules) == 0 {
		return raw, nil
	}
	x, err := parse(raw, false, "")
	if err != nil {
		return nil, fmt.Errorf("matchreplace: parse request: %w", err)
	}
	return e.run(ctx, rules, x, nil)
}

// Response returns the rewritten response, or resp itself when nothing
// changed; req is the request as sent upstream. Errors as for Request.
func (e *Engine) Response(ctx context.Context, source string, req, resp []byte) ([]byte, error) {
	rules := e.active(source, true)
	if len(rules) == 0 {
		return resp, nil
	}
	r, err := parse(req, false, "")
	if err != nil {
		return nil, fmt.Errorf("matchreplace: parse request: %w", err)
	}
	x, err := parse(resp, true, r.req.Method())
	if err != nil {
		return nil, fmt.Errorf("matchreplace: parse response: %w", err)
	}
	if strings.EqualFold(r.req.Method(), "CONNECT") && x.res.Status()/100 == 2 {
		return resp, nil // a tunnel opening is never rewritten (F-INT-1)
	}
	return e.run(ctx, rules, x, r)
}

func (e *Engine) run(ctx context.Context, rules []*compiled, x *msg, req *msg) ([]byte, error) {
	raw := x.raw
	if len(x.body) > captureCap {
		e.log.Info("matchreplace: body over the capture cap; no rule applied")
		return raw, nil
	}
	for _, c := range rules {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		next, _, err := step(c, x, req, proxyCap)
		if err != nil {
			e.log.Warn("matchreplace: rule skipped", "rule", c.name, "error", err)
			continue
		}
		if next != nil {
			x = next
		}
	}
	if bytes.Equal(x.raw, raw) {
		return raw, nil
	}
	return x.raw, nil
}

// errInvalid means a rule's result is not a whole message of the same kind.
var errInvalid = errors.New("matchreplace: result is not a valid message")

// step runs one rule on x: condition, then edit, then a re-parse of the
// result. next is nil when the rule changed nothing.
func step(c *compiled, x, req *msg, limit int) (next *msg, matched bool, err error) {
	v := view{req: x}
	if x.resp {
		v = view{req: req, resp: x}
	}
	if matched, err = eval(c.cond, v); err != nil || !matched {
		return nil, false, err
	}
	out, err := c.apply(x, limit)
	if err != nil || out == nil || bytes.Equal(out, x.raw) {
		return nil, true, err
	}
	if len(out) > limit {
		return nil, true, errTooLarge
	}
	if next, err = parse(out, x.resp, x.method); err != nil {
		return nil, true, fmt.Errorf("%w: %w", errInvalid, err)
	}
	return next, true, nil
}
