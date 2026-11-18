package omni

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type captureRunner struct {
	res string
	src string
}

func (c *captureRunner) Run(_ context.Context, src string) (string, error) {
	c.src = src
	return c.res, nil
}

func TestListTasksDecodes(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a1","name":"X","status":"Available","available":true,"tags":[{"id":"t1","name":"home"}],"project":{"id":"p1","name":"P"},"due":"2026-11-18T08:00:00"}]`}
	yes := true
	got, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{Available: &yes, Tag: "home", DueBefore: "2026-11-20"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Project.Name != "P" || got[0].Tags[0].ID != "t1" || got[0].Due != "2026-11-18T08:00:00" || !got[0].Available {
		t.Fatalf("bad decode %+v", got)
	}
}

func TestTaskDecodesEstimateAndRepetition(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","name":"A","status":"Available","tags":[],"estimatedMinutes":25,"repetition":{"rule":"FREQ=WEEKLY;INTERVAL=1;BYDAY=FR","scheduleType":"FromCompletion","anchor":"DeferDate","catchUp":true}},{"id":"b","name":"B","status":"Available","tags":[]}]`}
	got, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if got[0].EstimatedMinutes == nil || *got[0].EstimatedMinutes != 25 || got[0].Repetition == nil || *got[0].Repetition != (Repetition{Rule: "FREQ=WEEKLY;INTERVAL=1;BYDAY=FR", ScheduleType: "FromCompletion", Anchor: "DeferDate", CatchUp: true}) {
		t.Fatalf("bad decode %+v", got[0])
	}
	if got[1].EstimatedMinutes != nil || got[1].Repetition != nil {
		t.Fatalf("want omitted fields %+v", got[1])
	}
}

func TestTaskDecodesGroupFlagsAndDates(t *testing.T) {
	r := &captureRunner{res: `[{"id":"a","name":"A","status":"Dropped","tags":[],"hasChildren":true,"sequential":true,"completedByChildren":true,"completedAt":"2026-11-18T10:00:00","dropDate":"2026-11-18T11:00:00","added":"2026-11-01T08:00:00","modified":"2026-11-17T09:00:00"},{"id":"b","name":"B","status":"Available","tags":[]}]`}
	got, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	a := got[0]
	if !a.HasChildren || !a.Sequential || !a.CompletedByChildren || a.CompletedAt != "2026-11-18T10:00:00" || a.DropDate != "2026-11-18T11:00:00" || a.Added != "2026-11-01T08:00:00" || a.Modified != "2026-11-17T09:00:00" {
		t.Fatalf("bad decode %+v", a)
	}
	if b := got[1]; b.HasChildren || b.Sequential || b.CompletedByChildren || b.DropDate != "" {
		t.Fatalf("want zero values %+v", b)
	}
	raw, err := json.Marshal(got[1])
	if err != nil || strings.Contains(string(raw), "dropDate") || !strings.Contains(string(raw), `"hasChildren":false`) {
		t.Fatalf("dropDate must be omitted, flags always present: %s %v", raw, err)
	}
}

func TestRepetitionCheck(t *testing.T) {
	ok := Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: "DueDate"}
	if err := ok.check(); err != nil {
		t.Fatal(err)
	}
	for _, st := range repetitionScheduleTypes {
		if err := (Repetition{Rule: "FREQ=DAILY", ScheduleType: st, Anchor: "PlannedDate", CatchUp: true}).check(); err != nil {
			t.Errorf("%s: %v", st, err)
		}
	}
	for _, a := range repetitionAnchors {
		if err := (Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: a}).check(); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	for name, tc := range map[string]struct {
		r    Repetition
		want string
	}{
		"empty rule":     {Repetition{Rule: " ", ScheduleType: "Regularly", Anchor: "DueDate"}, "repetition.rule"},
		"missing type":   {Repetition{Rule: "FREQ=DAILY", Anchor: "DueDate"}, "repetition.scheduleType"},
		"None type":      {Repetition{Rule: "FREQ=DAILY", ScheduleType: "None", Anchor: "DueDate"}, "repetition.scheduleType"},
		"lowercase type": {Repetition{Rule: "FREQ=DAILY", ScheduleType: "regularly", Anchor: "DueDate"}, "repetition.scheduleType"},
		"missing anchor": {Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly"}, "repetition.anchor"},
		"unknown anchor": {Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: "StartDate"}, "repetition.anchor"},
	} {
		if err := tc.r.check(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error naming %s, got %v", name, tc.want, err)
		}
	}
	err := Repetition{}.check()
	for _, want := range []string{"repetition.rule", "repetition.scheduleType", "repetition.anchor"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("zero value must report every bad field, missing %s: %v", want, err)
		}
	}
}

func TestCheckEstimate(t *testing.T) {
	if checkEstimate(0) != nil || checkEstimate(90) != nil {
		t.Error("0 and positive minutes are valid")
	}
	if checkEstimate(-1) == nil {
		t.Error("negative minutes must be rejected")
	}
}

func TestListTasksRejectsBadInput(t *testing.T) {
	b := &Bridge{Runner: &captureRunner{res: `[]`}}
	neg := -1
	for _, f := range []TaskFilter{{DueBefore: "tomorrow"}, {DeferBefore: "2026-13-01"}, {Limit: &neg}} {
		if _, err := b.ListTasks(context.Background(), f); err == nil {
			t.Errorf("%+v: want error", f)
		}
	}
}

func TestListProjectsStatusValidation(t *testing.T) {
	r := &captureRunner{res: `[{"id":"p1","name":"P","status":"Active","tags":[],"folder":{"id":"f1","name":"F"}}]`}
	b := &Bridge{Runner: r}
	got, err := b.ListProjects(context.Background(), ProjectFilter{Status: "Active", Folder: "F"})
	if err != nil || len(got) != 1 || got[0].Folder.ID != "f1" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if _, err := b.ListProjects(context.Background(), ProjectFilter{Status: "Bogus"}); err == nil {
		t.Fatal("want status error")
	}
}

func TestListTagsAndFolders(t *testing.T) {
	tags, err := (&Bridge{Runner: &captureRunner{res: `[{"id":"t1","name":"a","status":"Active","remainingTasks":3}]`}}).ListTags(context.Background())
	if err != nil || len(tags) != 1 || tags[0].RemainingTasks != 3 {
		t.Fatalf("%+v %v", tags, err)
	}
	fs, err := (&Bridge{Runner: &captureRunner{res: `[{"id":"f1","name":"a","status":"Active","parent":{"id":"f0","name":"r"}}]`}}).ListFolders(context.Background())
	if err != nil || len(fs) != 1 || fs[0].Parent.ID != "f0" {
		t.Fatalf("%+v %v", fs, err)
	}
}

func TestGetters(t *testing.T) {
	ctx := context.Background()
	r := &captureRunner{res: `[{"id":"a1","name":"X","status":"Active","tags":[]}]`}
	b := &Bridge{Runner: r}
	if tk, err := b.GetTask(ctx, "a1"); err != nil || tk.ID != "a1" || !strings.Contains(r.src, `\"id\":\"a1\"`) {
		t.Fatalf("task %+v %v src %s", tk, err, r.src)
	}
	if p, err := b.GetProject(ctx, "a1"); err != nil || p.ID != "a1" {
		t.Fatalf("project %+v %v", p, err)
	}
	if g, err := b.GetTag(ctx, "a1"); err != nil || g.ID != "a1" {
		t.Fatalf("tag %+v %v", g, err)
	}
	if f, err := b.GetFolder(ctx, "a1"); err != nil || f.ID != "a1" {
		t.Fatalf("folder %+v %v", f, err)
	}
	empty := &Bridge{Runner: &captureRunner{res: `[]`}}
	if _, err := empty.GetTask(ctx, "zz"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := empty.GetFolder(ctx, ""); err == nil {
		t.Fatal("want id required error")
	}
}

func TestProjectDecodesReview(t *testing.T) {
	r := &captureRunner{res: `[{"id":"p1","name":"P","status":"Active","tags":[],"reviewInterval":{"steps":2,"unit":"weeks"},"nextReviewDate":"2026-12-02T00:00:00","lastReviewDate":"2026-11-18T09:30:00"},{"id":"p2","name":"Q","status":"Active","tags":[]}]`}
	got, err := (&Bridge{Runner: r}).ListProjects(context.Background(), ProjectFilter{ReviewDueBefore: "2026-12-02"})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	if ri := got[0].ReviewInterval; ri == nil || ri.Steps != 2 || ri.Unit != "weeks" || got[0].NextReview != "2026-12-02T00:00:00" || got[0].LastReview != "2026-11-18T09:30:00" {
		t.Fatalf("bad decode %+v", got[0])
	}
	if got[1].ReviewInterval != nil || got[1].NextReview != "" || got[1].LastReview != "" {
		t.Fatalf("want omitted review fields %+v", got[1])
	}
	if !strings.Contains(r.src, `\"review_due_before\":\"2026-12-02\"`) {
		t.Errorf("filter not passed through: %s", r.src)
	}
}

func TestListProjectsRejectsBadReviewDate(t *testing.T) {
	r := &captureRunner{res: `[]`}
	if _, err := (&Bridge{Runner: r}).ListProjects(context.Background(), ProjectFilter{ReviewDueBefore: "soon"}); err == nil || r.src != "" {
		t.Fatalf("want validation error before any script, got %v src=%q", err, r.src)
	}
}
