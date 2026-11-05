package fault

import (
	"errors"
	"fmt"
	"testing"
)

func TestOf(t *testing.T) {
	errGone := New(NotFound, "gone")
	errTaken := Coded(Conflict, "nameTaken", "name taken")
	for _, tc := range []struct {
		err        error
		kind       Kind
		code       string
		found      bool
		field      Field
		fieldFound bool
	}{
		{fmt.Errorf("pkg: get: %w", errGone), NotFound, "notFound", true, Field{}, false},
		{fmt.Errorf("pkg: add: %w", errTaken), Conflict, "nameTaken", true, Field{}, false},
		{fmt.Errorf("pkg: add: %w", About(fmt.Errorf("%w: %q", errTaken, "x"), Field{"/name", "invalid"})), Conflict, "nameTaken", true, Field{"/name", "invalid"}, true},
		{errors.New("plain"), 0, "", false, Field{}, false},
	} {
		f, ok := Of(tc.err)
		if ok != tc.found || ok && (f.Kind() != tc.kind || f.Code() != tc.code) {
			t.Errorf("%v: %v %v", tc.err, f, ok)
		}
		field, ok := FieldOf(tc.err)
		if ok != tc.fieldFound || field != tc.field {
			t.Errorf("%v: field %v %v", tc.err, field, ok)
		}
	}
	if err := About(errGone, Field{"id", "invalid"}); !errors.Is(err, errGone) || err.Error() != "gone" {
		t.Errorf("about: %v", err)
	}
}
