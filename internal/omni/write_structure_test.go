package omni

import (
	"context"
	"encoding/json"
	"errors"
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
	rep := &Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: "DueDate"}
	for name, tc := range map[string]struct {
		a    StructAdd
		want string
	}{
		"bad kind":        {StructAdd{Kind: "task", Name: "x"}, "kind"},
		"empty name":      {StructAdd{Kind: "tag", Name: " "}, "name"},
		"folder status":   {StructAdd{Kind: "folder", Name: "x", Status: "Active"}, "status"},
		"tag done":        {StructAdd{Kind: "tag", Name: "x", Status: "Done"}, "status"},
		"project dropped": {StructAdd{Kind: "project", Name: "x", Status: "Dropped"}, "status"},
		"tag type":        {StructAdd{Kind: "tag", Name: "x", Type: "Sequential"}, "only for projects, not tag"},
		"folder note":     {StructAdd{Kind: "folder", Name: "x", Note: "n"}, "only for projects, not folder"},
		"tag tags":        {StructAdd{Kind: "tag", Name: "x", Tags: []string{"home"}}, "only for projects"},
		"folder repeat":   {StructAdd{Kind: "folder", Name: "x", Repetition: rep}, "only for projects"},
		"tag estimate":    {StructAdd{Kind: "tag", Name: "x", EstimatedMinutes: new(5)}, "only for projects"},
		"tag due":         {StructAdd{Kind: "tag", Name: "x", Due: "2026-11-20"}, "only for projects"},
		"tag flagged":     {StructAdd{Kind: "tag", Name: "x", Flagged: true}, "only for projects"},
		"tag cbc":         {StructAdd{Kind: "tag", Name: "x", CompletedByChildren: true}, "only for projects"},
		"bad type":        {StructAdd{Kind: "project", Name: "x", Type: "sequential"}, "type"},
		"bad defer":       {StructAdd{Kind: "project", Name: "x", Defer: "soon"}, "defer"},
		"bad due":         {StructAdd{Kind: "project", Name: "x", Due: "soon"}, "due"},
		"bad planned":     {StructAdd{Kind: "project", Name: "x", Planned: "soon"}, "planned"},
		"negative est":    {StructAdd{Kind: "project", Name: "x", EstimatedMinutes: new(-1)}, "estimatedMinutes"},
		"bad repetition":  {StructAdd{Kind: "project", Name: "x", Repetition: &Repetition{Rule: "FREQ=DAILY"}}, "repetition"},
		"unknown tag":     {StructAdd{Kind: "project", Name: "x", Tags: []string{"nope"}}, "unknown tag"},
		"ambiguous tag":   {StructAdd{Kind: "project", Name: "x", Tags: []string{"dup"}}, "ambiguous"},
	} {
		if _, err := validAdd(tc.a, testTags); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
	for _, typ := range projectTypes {
		if _, err := validAdd(StructAdd{Kind: "project", Name: "x", Type: typ}, nil); err != nil {
			t.Errorf("type %s: %v", typ, err)
		}
	}
	full, err := validAdd(StructAdd{Kind: "project", Name: "p",
		Type: "SingleActions", Note: "n", Flagged: true, Defer: "2026-11-19", Due: "2026-11-20", Planned: "2026-11-21",
		Tags: []string{"home", "work"}, EstimatedMinutes: new(0), Repetition: rep, CompletedByChildren: true}, testTags)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, full), `{"kind":"project","name":"p","type":"SingleActions","note":"n","flagged":true,"defer":"2026-11-19","due":"2026-11-20","planned":"2026-11-21","tags":["t1","t2"],"estimatedMinutes":0,"repetition":{"rule":"FREQ=DAILY","scheduleType":"Regularly","anchor":"DueDate","catchUp":false},"completedByChildren":true}`; got != want {
		t.Errorf("project add:\n got %s\nwant %s", got, want)
	}
	it, err := validAdd(StructAdd{Kind: "project", Name: "p", Parent: "f1", Status: "OnHold", Type: "Sequential"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := wire(t, it), `{"kind":"project","name":"p","parent":"f1","status":"OnHold","type":"Sequential"}`; got != want {
		t.Errorf("full add:\n got %s\nwant %s", got, want)
	}
	min, err := validAdd(StructAdd{Kind: "tag", Name: "g"}, nil)
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
		"scalars":      {`{"kind":"project","id":"a","name":"N","type":"SingleActions"}`, `{"kind":"project","id":"a","name":"N","type":"SingleActions"}`},
		"restore":      {`{"kind":"folder","id":"a","status":"Active"}`, `{"kind":"folder","id":"a","status":"Active"}`},
	} {
		it, err := validEdit(decodeStructEdit(t, tc.in), testTags)
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
		it, err := validEdit(decodeStructEdit(t, tc.in), testTags)
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
		_, err := validEdit(decodeStructEdit(t, tc.in), testTags)
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

func TestStructEditProjectWire(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"type":        {`{"kind":"project","id":"a","type":"Sequential"}`, `{"kind":"project","id":"a","type":"Sequential"}`},
		"single":      {`{"kind":"project","id":"a","type":"SingleActions"}`, `{"kind":"project","id":"a","type":"SingleActions"}`},
		"null clears": {`{"kind":"project","id":"a","note":null,"defer":null,"due":null,"planned":null,"estimatedMinutes":null,"repetition":null}`, `{"kind":"project","id":"a","note":null,"defer":null,"due":null,"planned":null,"estimatedMinutes":null,"repetition":null}`},
		"values": {
			`{"kind":"project","id":"a","note":"n","flagged":false,"defer":"2026-11-19","due":"2026-11-20","planned":"2026-11-21","estimatedMinutes":0,"repetition":` + repJSON + `,"completedByChildren":false}`,
			`{"kind":"project","id":"a","note":"n","flagged":false,"defer":"2026-11-19","due":"2026-11-20","planned":"2026-11-21","estimatedMinutes":0,"repetition":` + repJSON + `,"completedByChildren":false}`},
		"tags by name": {`{"kind":"project","id":"a","tags":{"add":["home","t2"],"remove":["t3"]}}`, `{"kind":"project","id":"a","tags":{"add":["t1","t2"],"remove":["t3"]}}`},
		"tags add":     {`{"kind":"project","id":"a","tags":{"add":["home"]}}`, `{"kind":"project","id":"a","tags":{"add":["t1"]}}`},
		"tags remove":  {`{"kind":"project","id":"a","tags":{"remove":["home"]}}`, `{"kind":"project","id":"a","tags":{"remove":["t1"]}}`},
	} {
		it, err := validEdit(decodeStructEdit(t, tc.in), testTags)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := wire(t, it); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, tc.want)
		}
	}
}

func TestStructEditProjectValidation(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"bad type":         {`{"kind":"project","id":"a","type":"sequential"}`, "type"},
		"empty date":       {`{"kind":"project","id":"a","due":""}`, "use null to clear"},
		"bad defer":        {`{"kind":"project","id":"a","defer":"soon"}`, "defer"},
		"bad due":          {`{"kind":"project","id":"a","due":"soon"}`, "due"},
		"bad planned":      {`{"kind":"project","id":"a","planned":"soon"}`, "planned"},
		"negative est":     {`{"kind":"project","id":"a","estimatedMinutes":-1}`, "estimatedMinutes"},
		"bad repetition":   {`{"kind":"project","id":"a","repetition":{"rule":"FREQ=DAILY","scheduleType":"None","anchor":"DueDate","catchUp":false}}`, "scheduleType"},
		"unknown tag":      {`{"kind":"project","id":"a","tags":{"add":["nope"]}}`, "unknown tag"},
		"add and remove":   {`{"kind":"project","id":"a","tags":{"add":["home"],"remove":["t1"]}}`, "both added and removed"},
		"empty tag edit":   {`{"kind":"project","id":"a","tags":{}}`, "changes nothing"},
		"tag type":         {`{"kind":"tag","id":"a","type":"Parallel"}`, "only for projects, not tag"},
		"tag note null":    {`{"kind":"tag","id":"a","note":null}`, "only for projects, not tag"},
		"tag flagged":      {`{"kind":"tag","id":"a","flagged":false}`, "only for projects"},
		"tag tags":         {`{"kind":"tag","id":"a","tags":{"add":["home"]}}`, "only for projects"},
		"tag estimate":     {`{"kind":"tag","id":"a","estimatedMinutes":null}`, "only for projects"},
		"tag repetition":   {`{"kind":"tag","id":"a","repetition":null}`, "only for projects"},
		"tag cbc":          {`{"kind":"tag","id":"a","completedByChildren":true}`, "only for projects"},
		"tag due":          {`{"kind":"tag","id":"a","due":"2026-11-20"}`, "only for projects"},
		"folder defer":     {`{"kind":"folder","id":"a","defer":null}`, "only for projects, not folder"},
		"folder planned":   {`{"kind":"folder","id":"a","planned":null}`, "only for projects, not folder"},
		"folder on hold":   {`{"kind":"folder","id":"a","status":"OnHold"}`, "not one of [Active] for a folder"},
		"folder dropped":   {`{"kind":"folder","id":"a","status":"Dropped"}`, "not one of [Active] for a folder"},
		"folder done":      {`{"kind":"folder","id":"a","status":"Done"}`, "status"},
		"project dropped":  {`{"kind":"project","id":"a","status":"Dropped"}`, "status"},
		"tag done":         {`{"kind":"tag","id":"a","status":"Done"}`, "status"},
		"old sequential":   {`{"kind":"project","id":"a","sequential":true}`, "changes nothing"},
		"project null tag": {`{"kind":"project","id":"a","tags":null}`, "changes nothing"},
	} {
		_, err := validEdit(decodeStructEdit(t, tc.in), testTags)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
}

func TestEditStructureProjectFieldsPerItem(t *testing.T) {
	r := &route{
		tags: `[{"id":"t1","name":"home"}]`,
		res:  `[{"id":"p","kind":"project","ok":true,"name":"P","status":"Active"},{"id":"f","kind":"folder","ok":true,"name":"F","status":"Active"}]`,
	}
	got, err := (&Bridge{Runner: r}).EditStructure(context.Background(), []StructEdit{
		decodeStructEdit(t, `{"kind":"project","id":"p","type":"SingleActions","note":null,"tags":{"add":["home"]}}`),
		decodeStructEdit(t, `{"kind":"tag","id":"t","note":"x"}`),
		decodeStructEdit(t, `{"kind":"folder","id":"f","status":"Active"}`),
		decodeStructEdit(t, `{"kind":"folder","id":"g","status":"OnHold"}`),
	})
	if err != nil || len(got) != 4 || !got[0].OK || got[1].OK || got[1].ID != "t" || !strings.Contains(got[1].Error, "only for projects") || !got[2].OK || got[3].OK || got[3].ID != "g" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{`\"type\":\"SingleActions\"`, `\"note\":null`, `\"tags\":{\"add\":[\"t1\"]}`, `\"kind\":\"folder\",\"id\":\"f\",\"status\":\"Active\"`} {
		if !strings.Contains(r.writeSrc, want) {
			t.Errorf("payload lacks %s: %s", want, r.writeSrc)
		}
	}
	for _, bad := range []string{`\"id\":\"t\"`, `\"id\":\"g\"`} {
		if strings.Contains(r.writeSrc, bad) {
			t.Errorf("rejected item reached the script: %s", r.writeSrc)
		}
	}
}

func TestAddStructureProjectFieldsPerItem(t *testing.T) {
	r := &route{tags: `[{"id":"t1","name":"home"}]`, res: `[{"id":"n","kind":"project","ok":true,"name":"P","status":"Active"}]`}
	got, err := (&Bridge{Runner: r}).AddStructure(context.Background(), []StructAdd{
		{Kind: "project", Name: "P", Type: "Sequential", Tags: []string{"home"}, EstimatedMinutes: new(20), Due: "2026-11-20"},
		{Kind: "folder", Name: "F", Type: "Parallel"},
	})
	if err != nil || len(got) != 2 || !got[0].OK || got[1].OK || !strings.Contains(got[1].Error, "only for projects") {
		t.Fatalf("%+v %v", got, err)
	}
	for _, want := range []string{`\"type\":\"Sequential\"`, `\"tags\":[\"t1\"]`, `\"estimatedMinutes\":20`, `\"due\":\"2026-11-20\"`} {
		if !strings.Contains(r.writeSrc, want) {
			t.Errorf("payload lacks %s: %s", want, r.writeSrc)
		}
	}
	if strings.Contains(r.writeSrc, `\"name\":\"F\"`) {
		t.Error("rejected item reached the script")
	}
}

func TestStructTagLookupOnlyWhenNeeded(t *testing.T) {
	r := &route{res: `[{"id":"p","kind":"project","ok":true,"name":"P","status":"Active"}]`}
	if _, err := (&Bridge{Runner: r}).EditStructure(context.Background(), []StructEdit{decodeStructEdit(t, `{"kind":"project","id":"p","type":"Parallel"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Bridge{Runner: r}).AddStructure(context.Background(), []StructAdd{{Kind: "project", Name: "P"}}); err != nil {
		t.Fatal(err)
	}
	if len(r.scripts) != 2 || r.scripts[0] != "write" || r.scripts[1] != "write" {
		t.Errorf("tags read without a tag reference: %v", r.scripts)
	}
	boom := errors.New("tags exploded")
	r = &route{tagsErr: boom}
	if _, err := (&Bridge{Runner: r}).EditStructure(context.Background(), []StructEdit{decodeStructEdit(t, `{"kind":"project","id":"p","tags":{"add":["home"]}}`)}); !errors.Is(err, boom) {
		t.Fatalf("edit: %v", err)
	}
	if _, err := (&Bridge{Runner: r}).AddStructure(context.Background(), []StructAdd{{Kind: "project", Name: "P", Tags: []string{"home"}}}); !errors.Is(err, boom) {
		t.Fatalf("add: %v", err)
	}
	for _, s := range r.scripts {
		if s == "write" {
			t.Fatal("write script ran after tag lookup failed")
		}
	}
}
