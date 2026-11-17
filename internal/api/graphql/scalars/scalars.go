// Package scalars marshals the custom GraphQL scalars of base.graphqls: Time,
// Bytes and Int64 (docs/03-architecture.md §6). gqlgen binds them through the
// Marshal*/Unmarshal* function pairs (see gqlgen.yml).
package scalars

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/99designs/gqlgen/graphql"
)

// MarshalTime writes t as RFC 3339 with nanoseconds, in UTC.
func MarshalTime(t time.Time) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, strconv.Quote(t.UTC().Format(time.RFC3339Nano)))
	})
}

// UnmarshalTime reads an RFC 3339 string with any offset.
func UnmarshalTime(v any) (time.Time, error) {
	s, ok := v.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("invalid Time: want an RFC 3339 string, got %T", v)
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid Time: %w", err)
	}

	return t, nil
}

// MarshalBytes writes b as standard base64 with padding.
func MarshalBytes(b []byte) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, `"`+base64.StdEncoding.EncodeToString(b)+`"`)
	})
}

// UnmarshalBytes reads standard base64 with padding.
func UnmarshalBytes(v any) ([]byte, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("invalid Bytes: want a base64 string, got %T", v)
	}

	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid Bytes: %w", err)
	}

	return b, nil
}

// MarshalInt64 writes n as a JSON string of its decimal form.
func MarshalInt64(n int64) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		_, _ = io.WriteString(w, `"`+strconv.FormatInt(n, 10)+`"`)
	})
}

// UnmarshalInt64 reads a decimal string or a JSON integer.
func UnmarshalInt64(v any) (int64, error) {
	switch v := v.(type) {
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid Int64: %w", err)
		}

		return n, nil
	// gqlgen decodes request variables with encoding/json UseNumber.
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid Int64: %w", err)
		}

		return n, nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("invalid Int64: want a decimal string or an integer, got %T", v)
	}
}
