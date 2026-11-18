package server

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gildesmarais/omnifocus-mcp/internal/omni"
)

// writeInputs are the Go input types of the write tools, by tool name.
var writeInputs = map[string]reflect.Type{
	"add_tasks":  reflect.TypeFor[addTasksIn](),
	"edit_tasks": reflect.TypeFor[editTasksIn](),
	"add":        reflect.TypeFor[addIn](),
	"edit":       reflect.TypeFor[editIn](),
	"drop":       reflect.TypeFor[refsIn](),
	"delete":     reflect.TypeFor[refsIn](),
}

// schemaFacts collects every enum list (as "a|b|c") and every minimum under v,
// a decoded JSON schema.
func schemaFacts(v any, enums map[string]bool, mins map[float64]bool) {
	switch v := v.(type) {
	case map[string]any:
		if list, ok := v["enum"].([]any); ok {
			var parts []string
			for _, x := range list {
				parts = append(parts, x.(string))
			}
			enums[strings.Join(parts, "|")] = true
		}
		if m, ok := v["minimum"].(float64); ok {
			mins[m] = true
		}
		for _, x := range v {
			schemaFacts(x, enums, mins)
		}
	case []any:
		for _, x := range v {
			schemaFacts(x, enums, mins)
		}
	}
}

// Every rule a closed-set or bounded type carries, reached from the input of a
// registered write tool, is in that tool's schema. The tools come from the
// running server and the rules from the types, so neither is a hand list of
// fields; a new write tool must be added to writeInputs or this fails.
func TestWriteToolSchemasCarryEveryTypeRule(t *testing.T) {
	res, err := connect(t, fakeRunner{`[]`}).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, tl := range res.Tools {
		if tl.Annotations != nil && tl.Annotations.ReadOnlyHint {
			continue
		}
		root, ok := writeInputs[tl.Name]
		if !ok {
			t.Errorf("write tool %s is not in writeInputs", tl.Name)
			continue
		}
		seen++
		raw, _ := json.Marshal(tl.InputSchema)
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		enums, mins := map[string]bool{}, map[float64]bool{}
		schemaFacts(doc, enums, mins)
		for typ, r := range omni.InputRules(root) {
			if r.Names != nil && !enums[strings.Join(r.Names, "|")] {
				t.Errorf("%s: schema lacks the enum of %s %v", tl.Name, typ, r.Names)
			}
			if r.Min != nil && !mins[float64(*r.Min)] {
				t.Errorf("%s: schema lacks the minimum of %s (%d)", tl.Name, typ, *r.Min)
			}
		}
	}
	if seen != len(writeInputs) {
		t.Errorf("writeInputs names %d tools, the server registers %d write tools", len(writeInputs), seen)
	}
}

// descriptions collects every description string under v, a decoded JSON schema.
func descriptions(v any, out *[]string) {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if s, ok := x.(string); ok && k == "description" {
				*out = append(*out, s)
				continue
			}
			descriptions(x, out)
		}
	case []any:
		for _, x := range v {
			descriptions(x, out)
		}
	}
}

// The schema's enum is the one owner of a closed set's values: no tool or
// field description may repeat one of them.
func TestDescriptionsDoNotRepeatEnumValues(t *testing.T) {
	res, err := connect(t, fakeRunner{`[]`}).ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, root := range writeInputs {
		for _, r := range omni.InputRules(root) {
			names = append(names, r.Names...)
		}
	}
	if len(names) == 0 {
		t.Fatal("no enum names found")
	}
	for _, tl := range res.Tools {
		texts := []string{tl.Description}
		raw, _ := json.Marshal(map[string]any{"in": tl.InputSchema, "out": tl.OutputSchema})
		var doc any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		descriptions(doc, &texts)
		for _, text := range texts {
			for _, n := range names {
				if regexp.MustCompile(`\b` + n + `\b`).MatchString(text) {
					t.Errorf("%s: description repeats the enum value %q: %q", tl.Name, n, text)
				}
			}
		}
	}
}
