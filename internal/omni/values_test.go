package omni

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// InputRules finds every rule-carrying type through pointers, slices, embedded
// groups and Nullable wrappers, and reports exactly the facts the types own.
func TestInputRulesReadTheTypes(t *testing.T) {
	names := func(r Rule) string { return strings.Join(r.Names, ",") }
	task := InputRules(reflect.TypeFor[TaskAdd]())
	if len(task) != 3 || names(task[reflect.TypeFor[ScheduleType]()]) != "Regularly,FromCompletion" || names(task[reflect.TypeFor[Anchor]()]) != "DeferDate,DueDate,PlannedDate" {
		t.Errorf("task add rules: %v", task)
	}
	if m := task[reflect.TypeFor[Minutes]()].Min; m == nil || *m != 0 {
		t.Errorf("minutes minimum: %v", m)
	}
	edit := InputRules(reflect.TypeFor[StructEdit]())
	if len(edit) != 6 || names(edit[reflect.TypeFor[ReviewUnit]()]) != "days,weeks,months,years" || names(edit[reflect.TypeFor[ProjectType]()]) != "Parallel,Sequential,SingleActions" {
		t.Errorf("structure edit rules: %v", edit)
	}
	if m := edit[reflect.TypeFor[Steps]()].Min; m == nil || *m != 1 || edit[reflect.TypeFor[Steps]()].Names != nil {
		t.Errorf("steps rule: %v", edit[reflect.TypeFor[Steps]()])
	}
	if got := InputRules(reflect.TypeFor[StructRef]()); len(got) != 0 {
		t.Errorf("an input without closed sets has no rules: %v", got)
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
