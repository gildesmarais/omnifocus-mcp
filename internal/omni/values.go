package omni

import (
	"fmt"
	"reflect"
	"slices"
)

// enum is a string type that owns its closed set of names. The set is the one
// owner: validation checks against it and the input schema enumerates it.
type enum interface {
	~string
	names() []string
}

// bounded is an integer type that owns its lower bound, the way enum owns its
// names: validation checks against it and the input schema carries it.
type bounded interface {
	~int
	minimum() int
}

// checkEnum rejects a name outside the type's closed set.
func checkEnum[T enum](field string, v T) error {
	if all := v.names(); !slices.Contains(all, string(v)) {
		return fmt.Errorf("%s: %q must be one of %v", field, string(v), all)
	}
	return nil
}

// checkBound rejects a value below the type's lower bound.
func checkBound[T bounded](field string, v T) error {
	if min := v.minimum(); int(v) < min {
		return fmt.Errorf("%s: %d must be >= %d", field, int(v), min)
	}
	return nil
}

// text is the value of an optional string field; unset is empty.
func text(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ScheduleType is how a repeat rule schedules the next occurrence. OmniFocus
// also has a None type, but creating a rule with it throws, so a write rejects it.
type ScheduleType string

func (ScheduleType) names() []string { return []string{"Regularly", "FromCompletion"} }

// Anchor is the date a repeat rule moves.
type Anchor string

func (Anchor) names() []string { return []string{"DeferDate", "DueDate", "PlannedDate"} }

// ReviewUnit is the unit of a review interval. OmniFocus also reports hours on
// read, but silently resets any unit outside this set to 1 week on write.
type ReviewUnit string

func (ReviewUnit) names() []string { return []string{"days", "weeks", "months", "years"} }

// ProjectType is Parallel, Sequential (actions run in order) or SingleActions
// (a list of unrelated actions).
type ProjectType string

func (ProjectType) names() []string { return []string{"Parallel", "Sequential", "SingleActions"} }

// Minutes is an estimate in minutes, 0 or more.
type Minutes int

func (Minutes) minimum() int { return 0 }

// Steps is how many units make up a review interval, 1 or more.
type Steps int

func (Steps) minimum() int { return 1 }

// Rule is what a write-input type enforces beyond its Go type, as plain facts:
// the allowed names of a closed string set, or the lower bound of a counted
// value. The same facts back validation, so the transport schema (built in
// internal/server) cannot drift from it.
type Rule struct {
	Names []string // allowed values; nil for a counted value
	Min   *int     // lower bound; nil for a closed set
}

// ruleOf reads the rule a type carries, if any, off its enum or bounded methods.
func ruleOf(rt reflect.Type) (Rule, bool) {
	v := reflect.New(rt).Elem().Interface()
	var r Rule
	if e, ok := v.(interface{ names() []string }); ok {
		r.Names = e.names()
	}
	if b, ok := v.(interface{ minimum() int }); ok {
		m := b.minimum()
		r.Min = &m
	}
	return r, r.Names != nil || r.Min != nil
}

// InputRules returns the rule of every closed-set or bounded type reachable
// from root (a write input), keyed by Go type so a renamed property keeps its
// rule. It discovers by walking the fields, so a new enum or bounded type is
// covered by being used.
func InputRules(root reflect.Type) map[reflect.Type]Rule {
	rules := map[reflect.Type]Rule{}
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
		if r, ok := ruleOf(rt); ok {
			rules[rt] = r
		}
		if rt.Kind() == reflect.Struct {
			for _, f := range reflect.VisibleFields(rt) {
				walk(f.Type)
			}
		}
	}
	walk(root)
	return rules
}
