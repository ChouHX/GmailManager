// Package matcher implements the subject/sender patterns used by watch tasks.
package matcher

import (
	"errors"
	"regexp"
	"strings"
)

// Pattern is a compiled match rule.
//
// Supported forms:
//   - plain text     → substring match
//   - text with `*`  → wildcard match (each `*` matches any run of characters)
//   - `re:<regexp>`  → regular expression match
//
// Matching is case-insensitive unless the pattern starts with `case:`.
type Pattern struct {
	raw   string
	expr  *regexp.Regexp
	parts []string // wildcard segments, in order
	sub   string
	mode  mode
	fold  bool // case-insensitive matching
}

type mode int

const (
	modeEmpty mode = iota
	modeSubstring
	modeWildcard
	modeRegexp
)

// Compile parses one pattern. An empty pattern matches everything.
func Compile(raw string) (Pattern, error) {
	p := Pattern{raw: strings.TrimSpace(raw), fold: true}
	if p.raw == "" {
		return p, nil
	}
	text := p.raw
	caseSensitive := false
	if strings.HasPrefix(strings.ToLower(text), "case:") {
		caseSensitive = true
		text = strings.TrimSpace(text[len("case:"):])
	}
	if strings.HasPrefix(strings.ToLower(text), "re:") {
		expr := strings.TrimSpace(text[len("re:"):])
		if expr == "" {
			return Pattern{}, errors.New("正则表达式为空")
		}
		if !caseSensitive {
			expr = "(?i)" + expr
		}
		p.fold = !caseSensitive
		re, err := regexp.Compile(expr)
		if err != nil {
			return Pattern{}, err
		}
		p.expr = re
		p.mode = modeRegexp
		return p, nil
	}
	p.fold = !caseSensitive
	if p.fold {
		text = strings.ToLower(text)
	}
	p.sub = text
	if strings.Contains(text, "*") {
		for _, seg := range strings.Split(text, "*") {
			if seg != "" {
				p.parts = append(p.parts, seg)
			}
		}
		if len(p.parts) == 0 { // pattern is only stars
			p.mode = modeEmpty
			return p, nil
		}
		p.mode = modeWildcard
		return p, nil
	}
	p.mode = modeSubstring
	return p, nil
}

// Empty reports whether the pattern matches every value.
func (p Pattern) Empty() bool { return p.mode == modeEmpty }

// Match reports whether value satisfies the pattern.
func (p Pattern) Match(value string) bool {
	switch p.mode {
	case modeEmpty:
		return true
	case modeRegexp:
		return p.expr.MatchString(value)
	}
	haystack := value
	if p.fold {
		haystack = strings.ToLower(value)
	}
	if p.mode == modeSubstring {
		return strings.Contains(haystack, p.sub)
	}
	// Wildcard: the segments must appear in order.
	idx := 0
	for _, seg := range p.parts {
		at := strings.Index(haystack[idx:], seg)
		if at < 0 {
			return false
		}
		idx += at + len(seg)
	}
	return true
}

// String returns the original pattern text.
func (p Pattern) String() string { return p.raw }

// Rule is a compiled subject + sender pair. Both must match when set.
type Rule struct {
	Subject Pattern
	From    Pattern
}

// CompileRule builds a rule from raw user input.
func CompileRule(subject, from string) (Rule, error) {
	s, err := Compile(subject)
	if err != nil {
		return Rule{}, err
	}
	f, err := Compile(from)
	if err != nil {
		return Rule{}, err
	}
	return Rule{Subject: s, From: f}, nil
}

// Empty reports whether the rule has no constraint at all.
func (r Rule) Empty() bool { return r.Subject.Empty() && r.From.Empty() }

// Match reports whether one message hits the rule.
func (r Rule) Match(subject, from string) bool {
	return r.Subject.Match(subject) && r.From.Match(from)
}
