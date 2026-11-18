package omni

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

func decodeStructEdit(t *testing.T, s string) StructEdit {
	t.Helper()
	var e StructEdit
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestStructAddValidation(t *testing.T) {
	for name, a := range map[string]StructAdd{
		"bad kind":        {Kind: "task", Name: "x"},
		"empty name":      {Kind: "tag", Name: " "},
		"folder status":   {Kind: "folder", Name: "x", Status: "Active"},
		"tag done":        {Kind: "tag", Name: "x", Status: "Done"},
		"project dropped": {Kind: "project", Name: "x", Status: "Dropped"},
		"tag sequential":  {Kind: "tag", Name: "x", Sequential: true},
	} {
		if _, err := validAdd(a); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	it, err := validAdd(StructAdd{Kind: "project", Name: "p", Parent: "f1", Status: "OnHold", Sequential: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, it), `{"kind":"project","name":"p","parent":"f1","status":"OnHold","sequential":true}`; got != want {
		t.Errorf("full add:\n got %s\nwant %s", got, want)
	}
	min, err := validAdd(StructAdd{Kind: "tag", Name: "g"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, min), `{"kind":"tag","name":"g"}`; got != want {
		t.Errorf("minimal add: got %s want %s", got, want)
	}
}

func TestStructEditWire(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"move to root": {`{"kind":"project","id":"a","move":{"parent":null},"status":"Done"}`, `{"kind":"project","id":"a","status":"Done","move":{"parent":null}}`},
		"move parent":  {`{"kind":"tag","id":"a","move":{"parent":"p"}}`, `{"kind":"tag","id":"a","move":{"parent":"p"}}`},
		"reorder":      {`{"kind":"tag","id":"a","reorder":{"after":"b"}}`, `{"kind":"tag","id":"a","reorder":{"after":"b"}}`},
		"reorder lead": {`{"kind":"folder","id":"a","reorder":{"before":"b"}}`, `{"kind":"folder","id":"a","reorder":{"before":"b"}}`},
		"scalars":      {`{"kind":"project","id":"a","name":"N","sequential":false}`, `{"kind":"project","id":"a","name":"N","sequential":false}`},
	} {
		it, err := validEdit(decodeStructEdit(t, tc.in))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestStructEditReviewWire(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"interval":    {`{"kind":"project","id":"a","reviewInterval":{"steps":2,"unit":"weeks"}}`, `{"kind":"project","id":"a","reviewInterval":{"steps":2,"unit":"weeks"}}`},
		"next date":   {`{"kind":"project","id":"a","nextReviewDate":"2026-12-01"}`, `{"kind":"project","id":"a","nextReviewDate":"2026-12-01"}`},
		"reviewed":    {`{"kind":"project","id":"a","reviewed":true}`, `{"kind":"project","id":"a","reviewed":true}`},
		"interval+rv": {`{"kind":"project","id":"a","reviewInterval":{"steps":1,"unit":"years"},"reviewed":true}`, `{"kind":"project","id":"a","reviewInterval":{"steps":1,"unit":"years"},"reviewed":true}`},
	} {
		it, err := validEdit(decodeStructEdit(t, tc.in))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestStructEditReviewValidation(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"zero steps":      {`{"kind":"project","id":"a","reviewInterval":{"steps":0,"unit":"days"}}`, "steps"},
		"negative steps":  {`{"kind":"project","id":"a","reviewInterval":{"steps":-1,"unit":"days"}}`, "steps"},
		"missing steps":   {`{"kind":"project","id":"a","reviewInterval":{"unit":"days"}}`, "steps"},
		"singular unit":   {`{"kind":"project","id":"a","reviewInterval":{"steps":1,"unit":"week"}}`, "unit"},
		"unknown unit":    {`{"kind":"project","id":"a","reviewInterval":{"steps":1,"unit":"hours"}}`, "unit"},
		"missing unit":    {`{"kind":"project","id":"a","reviewInterval":{"steps":1}}`, "unit"},
		"bad date":        {`{"kind":"project","id":"a","nextReviewDate":"soon"}`, "nextReviewDate"},
		"reviewed + next": {`{"kind":"project","id":"a","reviewed":true,"nextReviewDate":"2026-12-01"}`, "mutually exclusive"},
		"null interval":   {`{"kind":"project","id":"a","reviewInterval":null}`, "changes nothing"},
		"interval on tag": {`{"kind":"tag","id":"a","reviewInterval":{"steps":1,"unit":"days"}}`, "only for projects"},
		"next on folder":  {`{"kind":"folder","id":"a","nextReviewDate":"2026-12-01"}`, "only for projects"},
		"reviewed on tag": {`{"kind":"tag","id":"a","reviewed":true}`, "only for projects"},
		"tag with name":   {`{"kind":"tag","id":"a","name":"x","reviewed":true}`, "only for projects"},
	} {
		_, err := validEdit(decodeStructEdit(t, tc.in))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
}

func TestEditReviewRejectsPerItemAndKeepsOthers(t *testing.T) {
	r := &captureRunner{res: `[{"id":"p","kind":"project","ok":true,"name":"P","status":"Active"}]`}
	got, err := (&Bridge{Runner: r}).EditStructure(context.Background(), []StructEdit{
		decodeStructEdit(t, `{"kind":"tag","id":"t","reviewed":true}`),
		decodeStructEdit(t, `{"kind":"project","id":"p","reviewInterval":{"steps":2,"unit":"weeks"},"reviewed":true}`),
	})
	if err != nil || len(got) != 2 || got[0].OK || got[0].ID != "t" || !got[1].OK {
		t.Fatalf("%+v %v", got, err)
	}
	if !strings.Contains(r.src, `\"reviewInterval\":{\"steps\":2,\"unit\":\"weeks\"}`) || !strings.Contains(r.src, `\"reviewed\":true`) || strings.Contains(r.src, `\"id\":\"t\"`) {
		t.Errorf("bad payload: %s", r.src)
	}
}

func TestStructRefWire(t *testing.T) {
	it, err := validRef(StructRef{Kind: "task", ID: "a"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, it), `{"kind":"task","id":"a"}`; got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

// Kind and Status are named types but must stay plain strings in the tool
// schema: an enum would reject the whole call instead of one item.
func TestNamedTypesStayStringSchemas(t *testing.T) {
	ref, err := jsonschema.For[StructRef](nil)
	if err != nil {
		t.Fatal(err)
	}
	add, err := jsonschema.For[StructAdd](nil)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := jsonschema.For[StructEdit](nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, sch := range map[string]*jsonschema.Schema{"add": add, "edit": edit} {
		for _, f := range []string{"kind", "status"} {
			if p := sch.Properties[f]; p == nil || p.Type != "string" || len(p.Enum) != 0 {
				t.Errorf("%s.%s: want plain string, got %+v", name, f, p)
			}
		}
	}
	if p := ref.Properties["kind"]; p == nil || p.Type != "string" || len(p.Enum) != 0 {
		t.Errorf("ref.kind: want plain string, got %+v", p)
	}
}

func TestDropRejectsBadKindAndMergesResults(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","kind":"tag","ok":false,"error":"tag still has 2 remaining task(s)"},{"id":"c","kind":"project","ok":true,"name":"P","status":"Dropped"}]`}
	got, err := (&Bridge{Runner: r}).DropObjects(context.Background(), []StructRef{{Kind: "tag", ID: "a"}, {Kind: "bogus", ID: "b"}, {Kind: "project", ID: "c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].OK || !strings.Contains(got[0].Error, "remaining") || got[1].OK || got[1].ID != "b" || !got[2].OK || got[2].Status != "Dropped" {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(r.src, "bogus") {
		t.Error("invalid item reached the script")
	}
}

func TestStructBatchCountMismatchAndEmpty(t *testing.T) {
	b := &Bridge{Runner: &captureRunner{res: `[]`}}
	if _, err := b.DeleteObjects(context.Background(), []StructRef{{Kind: "tag", ID: "a"}}); err == nil {
		t.Fatal("want mismatch error")
	}
	if _, err := b.AddStructure(context.Background(), nil); err == nil {
		t.Fatal("want empty error")
	}
}

func TestAddStructureDecodesRecord(t *testing.T) {
	b := &Bridge{Runner: &captureRunner{res: `[{"id":"n1","kind":"folder","ok":true,"name":"F","status":"Active","parent":{"id":"p","name":"Top"}}]`}}
	got, err := b.AddStructure(context.Background(), []StructAdd{{Kind: "folder", Name: "F", Parent: "p"}})
	if err != nil || got[0].Parent == nil || got[0].Parent.Name != "Top" || got[0].Name != "F" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestEditStructureDecodesRecord(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","kind":"tag","ok":true,"name":"T","status":"OnHold"}]`}
	got, err := (&Bridge{Runner: r}).EditStructure(context.Background(), []StructEdit{
		decodeStructEdit(t, `{"kind":"tag","id":"a","status":"OnHold","reorder":{"after":"b"}}`),
	})
	if err != nil || len(got) != 1 || !got[0].OK || got[0].Name != "T" || got[0].Status != "OnHold" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{`\"reorder\":{\"after\":\"b\"}`, `\"status\":\"OnHold\"`} {
		if !strings.Contains(r.src, want) {
			t.Errorf("payload lacks %s: %s", want, r.src)
		}
	}
}

func TestDeleteObjectsMergesScriptAndLocalResults(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","kind":"tag","ok":true,"name":"T"},{"id":"c","kind":"project","ok":false,"error":"project has 2 remaining task(s)"}]`}
	got, err := (&Bridge{Runner: r}).DeleteObjects(context.Background(), []StructRef{{Kind: "tag", ID: "a"}, {Kind: "bogus", ID: "b"}, {Kind: "project", ID: "c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "a" || !got[0].OK || got[1].ID != "b" || got[1].OK || got[2].ID != "c" || got[2].OK || !strings.Contains(got[2].Error, "remaining") {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(r.src, "bogus") {
		t.Error("invalid item reached the script")
	}
}
