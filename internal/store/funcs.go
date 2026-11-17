package store

import (
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
	t, ok := asBytes(text)
	if !ok {
		return nil, nil
	}
	re, err := compileCached(p)
	if err != nil {
		return nil, err
	}
	return boolInt(re.Match(t)), nil
}

// containsFold reports whether needle occurs in hay, comparing ASCII
// case-insensitively. It does not allocate.
func containsFold(hay, needle []byte) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	for i := 0; i+n <= len(hay); i++ {
		j := 0
		for ; j < n; j++ {
			if lowerASCII(hay[i+j]) != lowerASCII(needle[j]) {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
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
