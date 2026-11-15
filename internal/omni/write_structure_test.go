package omni

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
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
	if err != nil || it["parent"] != "f1" || it["status"] != "OnHold" || it["sequential"] != true {
		t.Fatalf("%v %v", it, err)
	}
}

func TestStructEditValidation(t *testing.T) {
	bad := map[string]string{
		"no id":          `{"kind":"tag","name":"x"}`,
		"task kind":      `{"kind":"task","id":"a","name":"x"}`,
		"no change":      `{"kind":"tag","id":"a"}`,
		"empty name":     `{"kind":"tag","id":"a","name":""}`,
		"folder status":  `{"kind":"folder","id":"a","status":"Active"}`,
		"tag done":       `{"kind":"tag","id":"a","status":"Done"}`,
		"move no parent": `{"kind":"tag","id":"a","move":{}}`,
		"move empty":     `{"kind":"tag","id":"a","move":{"parent":""}}`,
		"move self":      `{"kind":"tag","id":"a","move":{"parent":"a"}}`,
		"move+reorder":   `{"kind":"tag","id":"a","move":{"parent":null},"reorder":{"after":"b"}}`,
		"reorder none":   `{"kind":"tag","id":"a","reorder":{}}`,
		"reorder both":   `{"kind":"tag","id":"a","reorder":{"before":"b","after":"c"}}`,
		"reorder self":   `{"kind":"tag","id":"a","reorder":{"before":"a"}}`,
		"seq on tag":     `{"kind":"tag","id":"a","sequential":true}`,
	}
	for name, js := range bad {
		if _, err := newStructEditItem(decodeStructEdit(t, js)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	it, err := newStructEditItem(decodeStructEdit(t, `{"kind":"project","id":"a","move":{"parent":null},"status":"Done"}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(it)
	if !strings.Contains(string(raw), `"move":{"parent":null}`) {
		t.Errorf("root move must keep explicit null: %s", raw)
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
