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

// wire returns the JSON a payload sends to its script: the contract with js/*.js.
func wire(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestAddItemWire(t *testing.T) {
	full, err := newAddItem(TaskAdd{Name: "n", Note: "x", Flagged: true, Defer: "2026-11-19", Due: "2026-11-20", Planned: "2026-11-21", Project: "p", Tags: []string{"home", "t2"}}, testTags)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, full), `{"name":"n","note":"x","flagged":true,"defer":"2026-11-19","due":"2026-11-20","planned":"2026-11-21","project":"p","tags":["t1","t2"]}`; got != want {
		t.Errorf("full add:\n got %s\nwant %s", got, want)
	}
	min, err := newAddItem(TaskAdd{Name: "n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, min), `{"name":"n"}`; got != want {
		t.Errorf("minimal add: got %s want %s", got, want)
	}
	sub, err := newAddItem(TaskAdd{Name: "n", Parent: "q"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, sub), `{"name":"n","parent":"q"}`; got != want {
		t.Errorf("subtask add: got %s want %s", got, want)
	}
}

func TestPatchItemWire(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"set and clear": {`{"id":"a","defer":null,"due":"2026-11-20","note":null}`, `{"id":"a","note":null,"defer":null,"due":"2026-11-20"}`},
		"note set":      {`{"id":"a","note":"hi"}`, `{"id":"a","note":"hi"}`},
		"note empty":    {`{"id":"a","note":""}`, `{"id":"a","note":""}`},
		"planned clear": {`{"id":"a","planned":null}`, `{"id":"a","planned":null}`},
		"scalars":       {`{"id":"a","name":"N","flagged":false,"complete":true}`, `{"id":"a","name":"N","flagged":false,"complete":true}`},
		"uncomplete":    {`{"id":"a","uncomplete":true}`, `{"id":"a","uncomplete":true}`},
		"tags":          {`{"id":"a","tags":{"add":["home","t2"],"remove":["t3"]}}`, `{"id":"a","tagsAdd":["t1","t2"],"tagsRemove":["t3"]}`},
		"tags add only": {`{"id":"a","tags":{"add":["home"]}}`, `{"id":"a","tagsAdd":["t1"]}`},
		"move inbox":    {`{"id":"a","move":{"inbox":true}}`, `{"id":"a","move":{"inbox":true}}`},
		"move project":  {`{"id":"a","move":{"project":"p"}}`, `{"id":"a","move":{"project":"p"}}`},
		"move parent":   {`{"id":"a","move":{"parent":"q"}}`, `{"id":"a","move":{"parent":"q"}}`},
	} {
		it, err := newPatchItem(decodeEdit(t, tc.in), testTags)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestNullableStringWire(t *testing.T) {
	if got := wire(t, NullableString{Set: true, Null: true}); got != "null" {
		t.Errorf("clear: %s", got)
	}
	if got := wire(t, NullableString{Set: true, Value: "x"}); got != `"x"` {
		t.Errorf("value: %s", got)
	}
	if !(NullableString{}).IsZero() || (NullableString{Set: true, Null: true}).IsZero() {
		t.Error("IsZero must mean unset")
	}
}

func TestPatchEmptyTagEditChangesNothing(t *testing.T) {
	// Tags given but empty change nothing.
	if _, err := newPatchItem(decodeEdit(t, `{"id":"a","tags":{}}`), nil); err == nil {
		t.Fatal("empty tag edit must change nothing")
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
	if ids := it.TagsAdd; len(ids) != 2 || ids[0] != "t1" || ids[1] != "t2" {
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

func TestAddTasksResolvesTagNameToID(t *testing.T) {
	r := &route{
		tags: `[{"id":"t1","name":"home","status":"Active","remainingTasks":0}]`,
		res:  `[{"id":"n","ok":true,"task":{"id":"n","name":"x","status":"Available","tags":[{"id":"t1","name":"home"}]}}]`,
	}
	got, err := (&Bridge{Runner: r}).AddTasks(context.Background(), []TaskAdd{{Name: "x", Tags: []string{"home"}}})
	if err != nil || len(got) != 1 || !got[0].OK {
		t.Fatalf("%+v %v", got, err)
	}
	if !strings.Contains(r.writeSrc, `\"tags\":[\"t1\"]`) || strings.Contains(r.writeSrc, "home") {
		t.Fatalf("add script must carry the tag id, not the name: %s", r.writeSrc)
	}
}
