package store

import (
	"bytes"
	"database/sql/driver"
	"fmt"
	"regexp"
	"sync"

	"modernc.org/sqlite"
)

// maxRegexpCache bounds the number of compiled patterns kept by regexp().
const maxRegexpCache = 256

var (
	reMu    sync.Mutex
	reCache = make(map[string]*regexp.Regexp, maxRegexpCache)
)

func compileCached(pattern string) (*regexp.Regexp, error) {
	reMu.Lock()
	re, ok := reCache[pattern]
	reMu.Unlock()
	if ok {
		return re, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("regexp: %w", err)
	}
	reMu.Lock()
	if len(reCache) >= maxRegexpCache {
		clear(reCache)
	}
	reCache[pattern] = re
	reMu.Unlock()
	return re, nil
}

// asBytes converts a driver value to bytes; ok is false for NULL / unsupported.
// TEXT values are copied; hot paths should avoid it.
func asBytes(v driver.Value) ([]byte, bool) {
	switch x := v.(type) {
	case []byte:
		return x, true
	case string:
		return []byte(x), true
	default:
		return nil, false
	}
}

// sqlRegexp implements regexp(pattern, text): RE2 search match. NULL in, NULL out.
func sqlRegexp(pattern, text driver.Value) (driver.Value, error) {
	p, ok := pattern.(string)
	if !ok {
		return nil, nil
	}
	var match func(*regexp.Regexp) bool
	switch x := text.(type) {
	case string:
		match = func(re *regexp.Regexp) bool { return re.MatchString(x) }
	case []byte:
		match = func(re *regexp.Regexp) bool { return re.Match(x) }
	default:
		return nil, nil
	}
	re, err := compileCached(p)
	if err != nil {
		return nil, err
	}
	return boolInt(match(re)), nil
}

// containsFold reports whether needle occurs in hay, comparing ASCII
// case-insensitively. It does not allocate.
func containsFold(hay, needle []byte) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	lo, up := lowerASCII(needle[0]), upperASCII(needle[0])
	for i := 0; i+n <= len(hay); {
		j := bytes.IndexByte(hay[i:len(hay)-n+1], lo)
		if lo != up {
			if k := bytes.IndexByte(hay[i:len(hay)-n+1], up); k >= 0 && (j < 0 || k < j) {
				j = k
			}
		}
		if j < 0 {
			return false
		}
		i += j
		m := 1
		for ; m < n; m++ {
			if lowerASCII(hay[i+m]) != lowerASCII(needle[m]) {
				break
			}
		}
		if m == n {
			return true
		}
		i++
	}
	return false
}

func upperASCII(b byte) byte {
	if 'a' <= b && b <= 'z' {
		return b - ('a' - 'A')
	}
	return b
}

func lowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

var registerFuncs = sync.OnceValue(func() error {
	if err := sqlite.RegisterDeterministicScalarFunction("regexp", 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return sqlRegexp(args[0], args[1])
		}); err != nil {
		return fmt.Errorf("register regexp: %w", err)
	}
	if err := sqlite.RegisterDeterministicScalarFunction("bcontains_fold", 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			h, ok1 := asBytes(args[0])
			n, ok2 := asBytes(args[1])
			if !ok1 || !ok2 {
				return nil, nil
			}
			return boolInt(containsFold(h, n)), nil
		}); err != nil {
		return fmt.Errorf("register bcontains_fold: %w", err)
	}
	return nil
})
