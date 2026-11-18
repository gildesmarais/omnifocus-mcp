package omni

import (
	"context"
	"encoding/json"
	"slices"
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

func TestValidTaskAddWire(t *testing.T) {
	full, err := validTaskAdd(TaskAdd{Name: "n", Note: new("x"), Flagged: new(true), Defer: new("2026-11-19"), Due: new("2026-11-20"), Planned: new("2026-11-21"), Project: "p", Tags: []string{"home", "t2"}}, testTags)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, full), `{"name":"n","project":"p","note":"x","flagged":true,"defer":"2026-11-19","due":"2026-11-20","planned":"2026-11-21","tags":["t1","t2"]}`; got != want {
		t.Errorf("full add:\n got %s\nwant %s", got, want)
	}
	min, err := validTaskAdd(TaskAdd{Name: "n"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, min), `{"name":"n"}`; got != want {
		t.Errorf("minimal add: got %s want %s", got, want)
	}
	sub, err := validTaskAdd(TaskAdd{Name: "n", Parent: "q"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, sub), `{"name":"n","parent":"q"}`; got != want {
		t.Errorf("subtask add: got %s want %s", got, want)
	}
}

const repJSON = `{"rule":"FREQ=WEEKLY;INTERVAL=1;BYDAY=FR","scheduleType":"Regularly","anchor":"DueDate","catchUp":true}`

func TestValidTaskAddWireTaskFields(t *testing.T) {
	zero := Minutes(0)
	for name, tc := range map[string]struct {
		in   TaskAdd
		want string
	}{
		"all": {TaskAdd{Name: "n", EstimatedMinutes: new(Minutes(45)), Repetition: &Repetition{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=FR", ScheduleType: "Regularly", Anchor: "DueDate", CatchUp: true}, Sequential: true, CompletedByChildren: new(true)},
			`{"name":"n","estimatedMinutes":45,"repetition":` + repJSON + `,"completedByChildren":true,"sequential":true}`},
		"explicit false is sent": {TaskAdd{Name: "n", Flagged: new(false), CompletedByChildren: new(false)}, `{"name":"n","flagged":false,"completedByChildren":false}`},
		"zero estimate is sent":  {TaskAdd{Name: "n", EstimatedMinutes: &zero}, `{"name":"n","estimatedMinutes":0}`},
		"catchUp false is sent": {TaskAdd{Name: "n", Repetition: &Repetition{Rule: "FREQ=DAILY", ScheduleType: "FromCompletion", Anchor: "DeferDate"}},
			`{"name":"n","repetition":{"rule":"FREQ=DAILY","scheduleType":"FromCompletion","anchor":"DeferDate","catchUp":false}}`},
	} {
		it, err := validTaskAdd(tc.in, nil)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestAddValidationTaskFields(t *testing.T) {
	neg := Minutes(-5)
	good := Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: "DueDate"}
	for name, a := range map[string]TaskAdd{
		"negative estimate":    {Name: "x", EstimatedMinutes: &neg},
		"empty rule":           {Name: "x", Repetition: &Repetition{ScheduleType: "Regularly", Anchor: "DueDate"}},
		"missing scheduleType": {Name: "x", Repetition: &Repetition{Rule: "FREQ=DAILY", Anchor: "DueDate"}},
		"None scheduleType":    {Name: "x", Repetition: &Repetition{Rule: "FREQ=DAILY", ScheduleType: "None", Anchor: "DueDate"}},
		"missing anchor":       {Name: "x", Repetition: &Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly"}},
		"invalid anchor":       {Name: "x", Repetition: &Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: "Nope"}},
	} {
		if _, err := validTaskAdd(a, nil); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := validTaskAdd(TaskAdd{Name: "x", EstimatedMinutes: new(Minutes(0)), Repetition: &good}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestPatchItemWireTaskFields(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"estimate":           {`{"id":"a","estimatedMinutes":30}`, `{"id":"a","estimatedMinutes":30}`},
		"estimate zero":      {`{"id":"a","estimatedMinutes":0}`, `{"id":"a","estimatedMinutes":0}`},
		"estimate clear":     {`{"id":"a","estimatedMinutes":null}`, `{"id":"a","estimatedMinutes":null}`},
		"repetition":         {`{"id":"a","repetition":` + repJSON + `}`, `{"id":"a","repetition":` + repJSON + `}`},
		"repetition clear":   {`{"id":"a","repetition":null}`, `{"id":"a","repetition":null}`},
		"flags true":         {`{"id":"a","sequential":true,"completedByChildren":true}`, `{"id":"a","sequential":true,"completedByChildren":true}`},
		"flags false":        {`{"id":"a","sequential":false,"completedByChildren":false}`, `{"id":"a","sequential":false,"completedByChildren":false}`},
		"restore":            {`{"id":"a","restore":true}`, `{"id":"a","restore":true}`},
		"restore uncomplete": {`{"id":"a","restore":true,"uncomplete":true}`, `{"id":"a","uncomplete":true,"restore":true}`},
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

func TestPatchValidationTaskFields(t *testing.T) {
	bad := map[string]string{
		"negative estimate":    `{"id":"a","estimatedMinutes":-1}`,
		"empty rule":           `{"id":"a","repetition":{"rule":"","scheduleType":"Regularly","anchor":"DueDate","catchUp":false}}`,
		"missing scheduleType": `{"id":"a","repetition":{"rule":"FREQ=DAILY","anchor":"DueDate","catchUp":false}}`,
		"None scheduleType":    `{"id":"a","repetition":{"rule":"FREQ=DAILY","scheduleType":"None","anchor":"DueDate","catchUp":false}}`,
		"missing anchor":       `{"id":"a","repetition":{"rule":"FREQ=DAILY","scheduleType":"Regularly","catchUp":false}}`,
		"invalid anchor":       `{"id":"a","repetition":{"rule":"FREQ=DAILY","scheduleType":"Regularly","anchor":"Whenever","catchUp":false}}`,
		"restore and complete": `{"id":"a","restore":true,"complete":true}`,
	}
	for name, js := range bad {
		if _, err := newPatchItem(decodeEdit(t, js), testTags); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	_, err := newPatchItem(decodeEdit(t, `{"id":"a","restore":true,"complete":true}`), nil)
	if err == nil || !strings.Contains(err.Error(), "restore") {
		t.Errorf("restore+complete error must name restore: %v", err)
	}
}

func TestPatchTaskFieldsCountAsChanges(t *testing.T) {
	for _, js := range []string{`{"id":"a","estimatedMinutes":0}`, `{"id":"a","sequential":false}`, `{"id":"a","completedByChildren":false}`, `{"id":"a","repetition":null}`, `{"id":"a","restore":true}`} {
		if _, err := newPatchItem(decodeEdit(t, js), nil); err != nil {
			t.Errorf("%s: %v", js, err)
		}
	}
}

func TestNullableWire(t *testing.T) {
	if got := wire(t, Nullable[Minutes]{Set: true, Null: true}); got != "null" {
		t.Errorf("clear: %s", got)
	}
	if got := wire(t, Nullable[Minutes]{Set: true, Value: 0}); got != "0" {
		t.Errorf("zero value: %s", got)
	}
	var n Nullable[Minutes]
	if err := json.Unmarshal([]byte(`7`), &n); err != nil || !n.Set || n.Null || n.Value != 7 {
		t.Errorf("value: %+v %v", n, err)
	}
	if err := json.Unmarshal([]byte(`null`), &n); err != nil || !n.Set || !n.Null || n.Value != 0 {
		t.Errorf("null must reset the value: %+v %v", n, err)
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
		"bad due":    {Name: "x", Due: new("2026-13-40")},
		"both":       {Name: "x", Project: "p", Parent: "q"},
		"tag":        {Name: "x", Tags: []string{"nope"}},
	} {
		if _, err := validTaskAdd(a, testTags); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := validTaskAdd(TaskAdd{Name: "x", Tags: []string{"home"}, Due: new("2026-11-20T08:00:00")}, testTags); err != nil {
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

func TestTaskFieldsReachScriptAndBadItemsStayOut(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}}]`}
	got, err := (&Bridge{Runner: r}).EditTasks(context.Background(), []TaskEdit{
		decodeEdit(t, `{"id":"a","estimatedMinutes":15,"repetition":`+repJSON+`,"sequential":true,"completedByChildren":true,"restore":true}`),
		decodeEdit(t, `{"id":"b","restore":true,"complete":true}`),
		decodeEdit(t, `{"id":"c","estimatedMinutes":-3}`),
	})
	if err != nil || len(got) != 3 || !got[0].OK || got[1].OK || got[1].ID != "b" || !strings.Contains(got[1].Error, "restore") || got[2].OK || !strings.Contains(got[2].Error, "estimatedMinutes") {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{`\"estimatedMinutes\":15`, `\"scheduleType\":\"Regularly\"`, `\"anchor\":\"DueDate\"`, `\"catchUp\":true`, `\"sequential\":true`, `\"completedByChildren\":true`, `\"restore\":true`} {
		if !strings.Contains(r.src, want) {
			t.Errorf("script payload lacks %s: %s", want, r.src)
		}
	}
	if strings.Contains(r.src, `\"id\":\"b\"`) || strings.Contains(r.src, `\"id\":\"c\"`) {
		t.Error("invalid items reached the script")
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

func TestResolveTagsPrefersIDOverNameCollision(t *testing.T) {
	tags := []Tag{{ID: "x", Name: "a"}, {ID: "y", Name: "x"}}
	ids, err := resolveTags([]string{"x"}, tags)
	if err != nil || len(ids) != 1 || ids[0] != "x" {
		t.Fatalf("%v %v", ids, err)
	}
}

func TestPatchEstimateValidatedOnlyWhenSetAndNotNull(t *testing.T) {
	if _, err := newPatchItem(decodeEdit(t, `{"id":"a","name":"n"}`), nil); err != nil {
		t.Fatalf("unset estimate: %v", err)
	}
	if _, err := newPatchItem(TaskEdit{ID: "a", EstimatedMinutes: Nullable[Minutes]{Set: true, Null: true, Value: -5}}, nil); err != nil {
		t.Fatalf("null estimate clears and is not checked: %v", err)
	}
	if _, err := newPatchItem(TaskEdit{ID: "a", EstimatedMinutes: Nullable[Minutes]{Set: true, Value: -5}}, nil); err == nil {
		t.Fatal("set negative estimate must fail")
	}
}

func TestEditTasksReadsTagsOnlyForNonEmptyTagEdits(t *testing.T) {
	ok := `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}}]`
	for name, tc := range map[string]struct {
		js       string
		wantRead bool
	}{
		"empty edit": {`{"id":"a","name":"N","tags":{}}`, false},
		"no edit":    {`{"id":"a","name":"N"}`, false},
		"add":        {`{"id":"a","tags":{"add":["home"]}}`, true},
		"remove":     {`{"id":"a","tags":{"remove":["home"]}}`, true},
	} {
		r := &route{tags: `[{"id":"t1","name":"home"}]`, res: ok}
		if _, err := (&Bridge{Runner: r}).EditTasks(context.Background(), []TaskEdit{decodeEdit(t, tc.js)}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if read := slices.Contains(r.scripts, "tags"); read != tc.wantRead {
			t.Errorf("%s: tags read = %v, want %v (%v)", name, read, tc.wantRead, r.scripts)
		}
	}
}
