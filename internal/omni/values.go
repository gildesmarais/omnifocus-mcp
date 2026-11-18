package omni

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/google/jsonschema-go/jsonschema"
)

// enum is a string type that owns its closed set of names. The set is the one
// owner: validation checks against it and the input schema enumerates it.
type enum interface {
	~string
	names() []string
}

// checkEnum rejects a name outside the type's closed set.
func checkEnum[T enum](field string, v T) error {
	if all := v.names(); !slices.Contains(all, string(v)) {
		return fmt.Errorf("%s: %q must be one of %v", field, string(v), all)
	}
	return nil
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

// Lower bounds of the counted values below, shared by validation and schema.
const (
	minMinutes = 0
	minSteps   = 1
)

// Minutes is an estimate in minutes, 0 or more.
type Minutes int

func (m Minutes) check() error {
	if m < minMinutes {
		return fmt.Errorf("estimatedMinutes: %d must be >= %d", int(m), minMinutes)
	}
	return nil
}

// Steps is how many units make up a review interval, 1 or more.
type Steps int

func (s Steps) check() error {
	if s < minSteps {
		return fmt.Errorf("reviewInterval.steps: %d must be >= %d", int(s), minSteps)
	}
	return nil
}

// InputSchemas gives the schema of every write-input type that carries a rule
// the validators enforce: the enum names and the lower bounds. Keyed by Go type
// (for jsonschema.ForOptions.TypeSchemas), so a renamed property keeps its rule
// and a violation fails at the schema before any script runs. It returns fresh
// schemas on every call.
func InputSchemas() map[reflect.Type]*jsonschema.Schema {
	atLeast := func(min float64) *jsonschema.Schema { return &jsonschema.Schema{Type: "integer", Minimum: &min} }
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[ScheduleType](): enumSchema[ScheduleType](),
		reflect.TypeFor[Anchor]():       enumSchema[Anchor](),
		reflect.TypeFor[ReviewUnit]():   enumSchema[ReviewUnit](),
		reflect.TypeFor[ProjectType]():  enumSchema[ProjectType](),
		reflect.TypeFor[Minutes]():      atLeast(minMinutes),
		reflect.TypeFor[Steps]():        atLeast(minSteps),
	}
}

func enumSchema[T enum]() *jsonschema.Schema {
	var zero T
	s := &jsonschema.Schema{Type: "string"}
	for _, n := range zero.names() {
		s.Enum = append(s.Enum, n)
	}
	return s
}
