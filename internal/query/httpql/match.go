package httpql

import "unicode/utf8"

// textish is a byte-indexable string or byte slice.
type textish interface{ ~string | ~[]byte }

// containsFold reports whether needle occurs in hay, ASCII case-insensitively.
// It mirrors both instr(lower(hay), lower(needle)) > 0 and store's
// bcontains_fold, which fold A-Z only.
func containsFold[T textish](hay, needle T) bool {
	n := len(needle)
	if n == 0 {
		return true
	}
	lo, up := lowerASCII(needle[0]), upperASCII(needle[0])
	// One pass: every haystack byte is inspected once for needle[0], so the
	// scan stays linear on attacker-controlled bodies.
	for i, last := 0, len(hay)-n; i <= last; i++ {
		if c := hay[i]; c != lo && c != up {
			continue
		}
		m := 1
		for ; m < n; m++ {
			if lowerASCII(hay[i+m]) != lowerASCII(needle[m]) {
				break
			}
		}
		if m == n {
			return true
		}
	}
	return false
}

// likeMatch reports whether s matches pattern with the SQLite LIKE semantics
// of docs/04 §3: '%' matches any run, '_' exactly one character, '\' escapes
// the next character, and ASCII letters compare case-insensitively.
func likeMatch(pattern string, s []byte) bool {
	var (
		pat, str         int
		starPat, starStr = -1, 0
	)
	for str < len(s) {
		if pat < len(pattern) {
			switch c := pattern[pat]; c {
			case '%':
				starPat, starStr = pat+1, str
				pat++
				continue
			case '_':
				_, size := utf8.DecodeRune(s[str:])
				pat++
				str += size
				continue
			case '\\':
				if pat+1 < len(pattern) && eqFold(pattern[pat+1], s[str]) {
					pat += 2
					str++
					continue
				}
			default:
				if eqFold(c, s[str]) {
					pat++
					str++
					continue
				}
			}
		}
		if starPat < 0 {
			return false
		}
		// Retry the text after the last '%' one character further along.
		_, size := utf8.DecodeRune(s[starStr:])
		starStr += size
		str = starStr
		pat = starPat
	}
	for ; pat < len(pattern); pat++ {
		if pattern[pat] != '%' {
			return false
		}
	}
	return true
}

// eqFold reports whether two bytes are equal ASCII case-insensitively.
func eqFold(a, b byte) bool { return lowerASCII(a) == lowerASCII(b) }

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

// asciiLower lowercases the ASCII letters of s, leaving every other byte as it
// is (unlike strings.ToLower, which rewrites invalid UTF-8).
func asciiLower(s string) string {
	hasUpper := false
	for i := range len(s) {
		if 'A' <= s[i] && s[i] <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return s
	}
	b := []byte(s)
	for i := range b {
		b[i] = lowerASCII(b[i])
	}
	return string(b)
}
