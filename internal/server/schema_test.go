package server

import (
	"context"
	"encoding/json"
	"reflect"
	"regexp"
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
