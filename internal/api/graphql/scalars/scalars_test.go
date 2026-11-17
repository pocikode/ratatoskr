package scalars_test

import (
	"bytes"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"

	"github.com/pocikode/ratatoskr/internal/api/graphql/scalars"
	"github.com/pocikode/ratatoskr/internal/apperr"
)

func marshal(m graphql.Marshaler) string {
	var buf bytes.Buffer
	m.MarshalGQL(&buf)

	return buf.String()
}

func TestTime(t *testing.T) {
	t.Parallel()

	in := time.Date(2026, 11, 18, 10, 30, 0, 123456789, time.FixedZone("WIB", 7*3600))

	got := marshal(scalars.MarshalTime(in))
	if want := `"2026-11-18T03:30:00.123456789Z"`; got != want {
		t.Errorf("MarshalTime = %s, want %s", got, want)
	}

	back, err := scalars.UnmarshalTime("2026-11-18T10:30:00.123456789+07:00")
	if err != nil || !back.Equal(in) {
		t.Errorf("UnmarshalTime = %v, %v; want %v", back, err, in)
	}

	for _, bad := range []any{"yesterday", 42, nil} {
		if _, err := scalars.UnmarshalTime(bad); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("UnmarshalTime(%#v) = %v, want a BAD_INPUT error", bad, err)
		}
	}
}

func TestBytes(t *testing.T) {
	t.Parallel()

	in := []byte{0, 0xff, 'G', 'E', 'T', '\r', '\n'}

	got := marshal(scalars.MarshalBytes(in))
	if want := `"AP9HRVQNCg=="`; got != want {
		t.Errorf("MarshalBytes = %s, want %s", got, want)
	}

	back, err := scalars.UnmarshalBytes("AP9HRVQNCg==")
	if err != nil || !bytes.Equal(back, in) {
		t.Errorf("UnmarshalBytes = %v, %v; want %v", back, err, in)
	}

	for _, bad := range []any{"not base64!", "AP9HRVQNCg", 1} {
		if _, err := scalars.UnmarshalBytes(bad); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("UnmarshalBytes(%#v) = %v, want a BAD_INPUT error", bad, err)
		}
	}
}

func TestInt64(t *testing.T) {
	t.Parallel()

	if got, want := marshal(scalars.MarshalInt64(math.MaxInt64)), `"9223372036854775807"`; got != want {
		t.Errorf("MarshalInt64 = %s, want %s", got, want)
	}

	for _, in := range []any{"9007199254740993", json.Number("9007199254740993"), int64(9007199254740993)} {
		n, err := scalars.UnmarshalInt64(in)
		if err != nil || n != 9007199254740993 {
			t.Errorf("UnmarshalInt64(%#v) = %d, %v", in, n, err)
		}
	}

	for _, bad := range []any{"1.5", "9223372036854775808", json.Number("1e3"), 1.5, nil} {
		if _, err := scalars.UnmarshalInt64(bad); apperr.CodeOf(err) != apperr.BadInput {
			t.Errorf("UnmarshalInt64(%#v) = %v, want a BAD_INPUT error", bad, err)
		}
	}
}
