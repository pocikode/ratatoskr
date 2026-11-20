// Package matchreplace is the Match & Replace engine (docs/01-feature-spec.md
// F-MR-1): it applies an ordered rule list to raw HTTP/1.x requests and
// responses. It has no storage and no API; the service (T-102) loads rules
// into an [Engine] and the proxy calls it through its Rewriter interface.
//
// Messages are parsed with internal/rawhttp and edited with rawhttp.Builder,
// never with net/http (ADR-0003).
package matchreplace

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/pocikode/ratatoskr/internal/apperr"
	"github.com/pocikode/ratatoskr/internal/filterpreset"
	"github.com/pocikode/ratatoskr/internal/query/core"
)

// Source is the traffic a rule applies to; values are the schema enum
// MatchReplaceSource. The proxy hooks pass lower-case names ("proxy"), which
// match case-insensitively.
type Source string

// Sources.
const (
	SourceProxy     Source = "PROXY"
	SourceIntercept Source = "INTERCEPT"
	SourceReplay    Source = "REPLAY"
	SourceAutomate  Source = "AUTOMATE"
	SourceWorkflow  Source = "WORKFLOW"
	SourcePlugin    Source = "PLUGIN"
)

// Section is the part of a message a rule works on (schema MatchReplaceSection).
type Section string

// Sections.
const (
	RequestFirstLine   Section = "REQUEST_FIRST_LINE"
	RequestMethod      Section = "REQUEST_METHOD"
	RequestPath        Section = "REQUEST_PATH"
	RequestQuery       Section = "REQUEST_QUERY"
	RequestHeader      Section = "REQUEST_HEADER"
	RequestBody        Section = "REQUEST_BODY"
	ResponseFirstLine  Section = "RESPONSE_FIRST_LINE"
	ResponseStatusCode Section = "RESPONSE_STATUS_CODE"
	ResponseHeader     Section = "RESPONSE_HEADER"
	ResponseBody       Section = "RESPONSE_BODY"
)

// Operation is what a rule does to its section (schema MatchReplaceOperation).
type Operation string

// Operations.
const (
	UpdateRaw   Operation = "UPDATE_RAW"
	Update      Operation = "UPDATE"
	UpdateValue Operation = "UPDATE_VALUE"
	Add         Operation = "ADD"
	Remove      Operation = "REMOVE"
)

// MatcherKind is how a rule finds what it changes (schema MatchReplaceMatcherKind).
type MatcherKind string

// Matcher kinds.
const (
	Full   MatcherKind = "FULL"
	String MatcherKind = "STRING"
	Regex  MatcherKind = "REGEX"
	Name   MatcherKind = "NAME"
)

// Kind is the message kind the tester runs on (schema MatchReplaceMessageKind).
type Kind string

// Message kinds.
const (
	KindRequest  Kind = "REQUEST"
	KindResponse Kind = "RESPONSE"
)

// Matcher is a rule's matcher.
type Matcher struct {
	Kind  MatcherKind `json:"kind"`
	Value string      `json:"value"`
}

// Rule mirrors MatchReplaceRuleInput (T-099), enum values as their schema names.
type Rule struct {
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Condition string    `json:"condition"`
	Sources   []Source  `json:"sources"`
	Section   Section   `json:"section"`
	Operation Operation `json:"operation"`
	Matcher   Matcher   `json:"matcher"`
	Term      string    `json:"term"`
}

// Limits from F-MR-1.
const (
	maxNameRunes = 200
	maxTermBytes = 1 << 20
)

// ops lists the valid operations per section.
var ops = map[Section][]Operation{
	RequestFirstLine:   {UpdateRaw},
	RequestPath:        {UpdateRaw},
	RequestBody:        {UpdateRaw},
	ResponseFirstLine:  {UpdateRaw},
	ResponseBody:       {UpdateRaw},
	RequestMethod:      {Update},
	ResponseStatusCode: {Update},
	RequestQuery:       {UpdateRaw, UpdateValue, Add, Remove},
	RequestHeader:      {UpdateRaw, UpdateValue, Add, Remove},
	ResponseHeader:     {UpdateRaw, UpdateValue, Add, Remove},
}

// matchers lists the matcher kinds per operation; Add ignores the matcher.
var matchers = map[Operation][]MatcherKind{
	UpdateRaw:   {Full, String, Regex},
	Update:      {Full, String, Regex},
	UpdateValue: {Full, String, Regex, Name},
	Remove:      {Full, String, Regex, Name},
}

var sources = []Source{SourceProxy, SourceIntercept, SourceReplay, SourceAutomate, SourceWorkflow, SourcePlugin}

// response reports whether s is a response section.
func (s Section) response() bool { return strings.HasPrefix(string(s), "RESPONSE_") }

func (s Section) header() bool { return s == RequestHeader || s == ResponseHeader }

// compiled is a rule ready to apply.
type compiled struct {
	name      string
	sources   []Source
	section   Section
	op        Operation
	kind      MatcherKind
	match     string // matcher value
	value     []byte
	term      []byte
	re        *regexp.Regexp
	cond      core.Node
	addHeader [2]string // name, value of a header ADD
}

// Validate checks a whole rule as create, update and import do: name, sources,
// then everything [Engine.Test] checks. Errors are *apperr.Error: BAD_INPUT
// naming the field, or BAD_QUERY with span for the condition.
func Validate(r Rule, presets []filterpreset.Preset) error {
	name := strings.TrimSpace(r.Name)
	if n := utf8.RuneCountInString(name); n < 1 || n > maxNameRunes {
		return apperr.New(apperr.BadInput, "name must be 1-%d characters", maxNameRunes)
	}
	if strings.ContainsFunc(name, unicode.IsControl) {
		return apperr.New(apperr.BadInput, "name must not contain control characters")
	}
	if len(r.Sources) == 0 {
		return apperr.New(apperr.BadInput, "sources must not be empty")
	}
	for _, s := range r.Sources {
		if !slices.Contains(sources, s) {
			return apperr.New(apperr.BadInput, "sources: unknown source %q", s)
		}
	}
	_, err := compile(r, presets)
	return err
}

// compile validates the fields a rule applies with (not name, enabled or
// sources) and prepares it.
func compile(r Rule, presets []filterpreset.Preset) (*compiled, error) {
	if _, ok := ops[r.Section]; !ok {
		return nil, apperr.New(apperr.BadInput, "section: unknown section %q", r.Section)
	}
	if !slices.Contains(ops[r.Section], r.Operation) {
		return nil, apperr.New(apperr.BadInput, "operation: %s is not valid for section %s", r.Operation, r.Section)
	}
	if len(r.Term) > maxTermBytes {
		return nil, apperr.New(apperr.BadInput, "term must be at most 1 MiB")
	}
	if len(r.Matcher.Value) > maxTermBytes {
		return nil, apperr.New(apperr.BadInput, "matcher.value must be at most 1 MiB")
	}
	if len(r.Condition) > core.MaxQueryLen {
		return nil, apperr.New(apperr.BadInput, "condition must be at most 64 KiB")
	}
	c := &compiled{
		name: r.Name, sources: slices.Clone(r.Sources), // the caller may reuse its slice after Set
		section: r.Section, op: r.Operation,
		kind: r.Matcher.Kind, match: r.Matcher.Value, value: []byte(r.Matcher.Value), term: []byte(r.Term),
	}
	if r.Operation == Add {
		if r.Section.header() {
			name, value, ok := strings.Cut(r.Term, ":")
			if !ok || !isToken(name) {
				return nil, apperr.New(apperr.BadInput, "term must be \"Name: value\" with a token name to add a header")
			}
			c.addHeader = [2]string{name, strings.TrimLeft(value, " \t")}
		}
	} else {
		if !slices.Contains(matchers[r.Operation], r.Matcher.Kind) {
			return nil, apperr.New(apperr.BadInput, "matcher.kind: %q is not allowed with %s", r.Matcher.Kind, r.Operation)
		}
		if r.Matcher.Kind != Full && r.Matcher.Value == "" {
			return nil, apperr.New(apperr.BadInput, "matcher.value must not be empty for %s", r.Matcher.Kind)
		}
		if r.Matcher.Kind == Regex {
			re, err := regexp.Compile(r.Matcher.Value)
			if err != nil {
				return nil, apperr.New(apperr.BadInput, "matcher.value: %v", err)
			}
			c.re = re
		}
	}
	cond, err := filterpreset.Expand(r.Condition, presets)
	if err != nil {
		return nil, err
	}
	if err := checkFields(cond); err != nil {
		return nil, err
	}
	c.cond = cond
	return c, nil
}

// applies reports whether c runs for source on the given side.
func (c *compiled) applies(source string, response bool) bool {
	if c.section.response() != response {
		return false
	}
	for _, s := range c.sources {
		if strings.EqualFold(string(s), source) {
			return true
		}
	}
	return false
}

// isToken reports whether s is an HTTP token (RFC 9110 §5.6.2).
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		b := s[i]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
			strings.IndexByte("!#$%&'*+-.^_`|~", b) >= 0 {
			continue
		}
		return false
	}
	return true
}
