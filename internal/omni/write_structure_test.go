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
		if _, err := newStructAddItem(a); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	it, err := newStructAddItem(StructAdd{Kind: "project", Name: "p", Parent: "f1", Status: "OnHold", Sequential: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, it), `{"kind":"project","name":"p","parent":"f1","status":"OnHold","sequential":true}`; got != want {
		t.Errorf("full add:\n got %s\nwant %s", got, want)
	}
	min, err := newStructAddItem(StructAdd{Kind: "tag", Name: "g"})
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
		it, err := newStructEditItem(decodeStructEdit(t, tc.in))
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestStructRefWire(t *testing.T) {
	it, err := newStructRefItem(StructRef{Kind: "task", ID: "a"})
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
