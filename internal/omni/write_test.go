package omni

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func decodeEdit(t *testing.T, s string) TaskEdit {
	t.Helper()
	var e TaskEdit
	if err := json.Unmarshal([]byte(s), &e); err != nil {
		t.Fatal(err)
	}
	return e
}

var testTags = []Tag{{ID: "t1", Name: "home"}, {ID: "t2", Name: "work"}, {ID: "t3", Name: "dup"}, {ID: "t4", Name: "dup"}}

func TestPatchNullVersusOmitted(t *testing.T) {
	it, err := newPatchItem(decodeEdit(t, `{"id":"a","defer":null,"due":"2026-11-20","note":null}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(it)
	got := string(raw)
	for _, want := range []string{`"defer":null`, `"note":null`, `"due":"2026-11-20"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
	if strings.Contains(got, "planned") || strings.Contains(got, "name") {
		t.Errorf("omitted fields must stay out: %s", got)
	}
}

func TestPatchValidation(t *testing.T) {
	bad := map[string]string{
		"bad date":         `{"id":"a","due":"tomorrow"}`,
		"empty date":       `{"id":"a","due":""}`,
		"empty name":       `{"id":"a","name":"  "}`,
		"no id":            `{"name":"x"}`,
		"no change":        `{"id":"a"}`,
		"complete both":    `{"id":"a","complete":true,"uncomplete":true}`,
		"two move targets": `{"id":"a","move":{"project":"p","inbox":true}}`,
		"empty move":       `{"id":"a","move":{}}`,
		"unknown tag":      `{"id":"a","tags":{"add":["nope"]}}`,
		"ambiguous tag":    `{"id":"a","tags":{"remove":["dup"]}}`,
		"add and remove":   `{"id":"a","tags":{"add":["home"],"remove":["t1"]}}`,
	}
	for name, js := range bad {
		if _, err := newPatchItem(decodeEdit(t, js), testTags); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	it, err := newPatchItem(decodeEdit(t, `{"id":"a","tags":{"add":["home","t2"]},"move":{"inbox":true}}`), testTags)
	if err != nil {
		t.Fatal(err)
	}
	if ids := it["tagsAdd"].([]string); len(ids) != 2 || ids[0] != "t1" || ids[1] != "t2" {
		t.Errorf("tags not resolved to ids: %v", ids)
	}
}

func TestAddValidation(t *testing.T) {
	for name, a := range map[string]TaskAdd{
		"empty name": {Name: " "},
		"bad due":    {Name: "x", Due: "2026-13-40"},
		"both":       {Name: "x", Project: "p", Parent: "q"},
		"tag":        {Name: "x", Tags: []string{"nope"}},
	} {
		if _, err := newAddItem(a, testTags); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := newAddItem(TaskAdd{Name: "x", Tags: []string{"home"}, Due: "2026-11-20T08:00:00"}, testTags); err != nil {
		t.Fatal(err)
	}
}

func TestEditTasksMergesPerItemResults(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}},{"id":"c","ok":false,"error":"task not found"}]`}
	b := &Bridge{Runner: r}
	patches := []TaskEdit{
		decodeEdit(t, `{"id":"a","name":"N"}`),
		decodeEdit(t, `{"id":"b","due":"nope"}`),
		decodeEdit(t, `{"id":"c","flagged":true}`),
	}
	got, err := b.EditTasks(context.Background(), patches)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[0].OK || got[0].Task.Name != "N" || got[1].OK || got[1].ID != "b" || !strings.Contains(got[1].Error, "due") || got[2].OK || got[2].Error != "task not found" {
		t.Fatalf("%+v", got)
	}
	if strings.Contains(r.src, `nope`) {
		t.Error("invalid item reached the script")
	}
}

func TestAllInvalidBatchRunsNoScript(t *testing.T) {
	r := &captureRunner{res: `[]`}
	got, err := (&Bridge{Runner: r}).AddTasks(context.Background(), []TaskAdd{{Name: ""}})
	if err != nil || len(got) != 1 || got[0].OK || r.src != "" {
		t.Fatalf("%+v %v src=%q", got, err, r.src)
	}
}

func TestScriptResultCountMismatch(t *testing.T) {
	b := &Bridge{Runner: &captureRunner{res: `[]`}}
	if _, err := b.AddTasks(context.Background(), []TaskAdd{{Name: "x"}}); err == nil {
		t.Fatal("want mismatch error")
	}
}
