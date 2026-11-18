package omni

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// Every closed-set type reachable from a write input is in InputSchemas, so no
// enum or lower bound can silently miss the tool schema.
func TestInputSchemasCoverEveryClosedSetField(t *testing.T) {
	registered := InputSchemas()
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice {
			rt = rt.Elem()
		}
		if seen[rt] {
			return
		}
		seen[rt] = true
		_, isEnum := reflect.New(rt).Elem().Interface().(interface{ names() []string })
		if isEnum || rt == reflect.TypeFor[Minutes]() || rt == reflect.TypeFor[Steps]() {
			if registered[rt] == nil {
				t.Errorf("%s is a closed-set input type missing from InputSchemas", rt)
			}
		}
		if rt.Kind() == reflect.Struct {
			for _, f := range reflect.VisibleFields(rt) {
				walk(f.Type)
			}
		}
	}
	for _, in := range []reflect.Type{reflect.TypeFor[TaskAdd](), reflect.TypeFor[TaskEdit](), reflect.TypeFor[StructAdd](), reflect.TypeFor[StructEdit]()} {
		walk(in)
	}
	for rt := range registered {
		if !seen[rt] {
			t.Errorf("%s is registered but no write input reaches it", rt)
		}
	}
}

func TestInputSchemasCarryTheRules(t *testing.T) {
	s := InputSchemas()
	if got := s[reflect.TypeFor[Anchor]()].Enum; len(got) != 3 || got[0] != "DeferDate" {
		t.Errorf("anchor enum: %v", got)
	}
	if m := s[reflect.TypeFor[Minutes]()].Minimum; m == nil || *m != 0 {
		t.Errorf("minutes minimum: %v", m)
	}
	if m := s[reflect.TypeFor[Steps]()].Minimum; m == nil || *m != 1 {
		t.Errorf("steps minimum: %v", m)
	}
	if err := checkEnum("unit", ReviewUnit("week")); err == nil || !strings.Contains(err.Error(), `unit: "week" must be one of [days weeks months years]`) {
		t.Errorf("enum error: %v", err)
	}
}

func TestDateBound(t *testing.T) {
	day, err := parseDate("due", "2026-11-18")
	if err != nil || !day.dayOnly {
		t.Fatalf("date-only value: %+v %v", day, err)
	}
	lo, hi := day.bound(false), day.bound(true)
	if lo.text != "2026-11-18T00:00:00" || hi.text != "2026-11-18T23:59:59" || !lo.t.Equal(day.t) || hi.t.Sub(lo.t) != 24*time.Hour-time.Second || lo.dayOnly || hi.dayOnly {
		t.Errorf("a day bounds to its first and last second: %+v %+v", lo, hi)
	}
	at, err := parseDate("due", "2026-11-18T08:30:00Z")
	if err != nil || at.dayOnly || at.bound(true) != at || at.bound(false) != at {
		t.Errorf("an instant is its own bound: %+v %v", at, err)
	}
	if none, err := parseDate("due", ""); err != nil || !none.IsZero() {
		t.Errorf("empty is the absent date: %+v %v", none, err)
	}
}
