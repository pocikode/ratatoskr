package httpql

import (
	"regexp"

	lru "github.com/hashicorp/golang-lru/v2"
)

// regexpCacheSize bounds the compiled-pattern cache (docs/04 §4).
const regexpCacheSize = 256

// regexps caches compiled patterns; lru.Cache is safe for concurrent use.
var regexps = newRegexpCache()

func newRegexpCache() *lru.Cache[string, *regexp.Regexp] {
	// The only error is a non-positive size, and the size is a constant.
	c, _ := lru.New[string, *regexp.Regexp](regexpCacheSize)
	return c
}

// regexpMatch reports whether pattern matches s. Patterns reach the evaluator
// already compiled once by the parser; an invalid one cannot match.
func regexpMatch(pattern string, s []byte) bool {
	re, ok := regexps.Get(pattern)
	if !ok {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			return false
		}
		regexps.Add(pattern, compiled)
		re = compiled
	}
	return re.Match(s)
}
