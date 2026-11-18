package omni

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
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
	for _, st := range ScheduleType("").names() {
		if err := (Repetition{Rule: "FREQ=DAILY", ScheduleType: ScheduleType(st), Anchor: "PlannedDate", CatchUp: true}).check(); err != nil {
			t.Errorf("%s: %v", st, err)
		}
	}
	for _, a := range Anchor("").names() {
		if err := (Repetition{Rule: "FREQ=DAILY", ScheduleType: "Regularly", Anchor: Anchor(a)}).check(); err != nil {
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
	if checkBound("m", Minutes(0)) != nil || checkBound("m", Minutes(90)) != nil {
		t.Error("0 and positive minutes are valid")
	}
	if err := checkBound("estimatedMinutes", Minutes(-1)); err == nil || err.Error() != "estimatedMinutes: -1 must be >= 0" {
		t.Errorf("negative minutes must be rejected naming the field: %v", err)
	}
	if err := checkBound("reviewInterval.steps", Steps(0)); err == nil || err.Error() != "reviewInterval.steps: 0 must be >= 1" {
		t.Errorf("steps below 1 must be rejected naming the field: %v", err)
	}
}

func TestListTasksRejectsBadInput(t *testing.T) {
	r := &captureRunner{res: `[]`}
	b := &Bridge{Runner: r}
	neg := -1
	for _, f := range []TaskFilter{
		{DueBefore: "tomorrow"}, {DeferBefore: "2026-13-01"}, {Limit: &neg},
		{DueAfter: "x"}, {DeferAfter: "x"}, {PlannedBefore: "x"}, {PlannedAfter: "x"},
		{CompletedBefore: "x"}, {CompletedAfter: "x"}, {ModifiedAfter: "x"}, {AddedAfter: "x"},
	} {
		if _, err := b.ListTasks(context.Background(), f); err == nil {
			t.Errorf("%+v: want error", f)
		}
	}
	if r.src != "" {
		t.Fatalf("a script ran for invalid input: %s", r.src)
	}
}

func TestCheckLimit(t *testing.T) {
	zero, neg := 0, -1
	if err := checkLimit(&zero); err != nil {
		t.Errorf("limit 0 is valid: %v", err)
	}
	if err := checkLimit(nil); err != nil {
		t.Errorf("nil limit is valid: %v", err)
	}
	if err := checkLimit(&neg); err == nil {
		t.Error("negative limit must be rejected")
	}
}

func TestResolveBound(t *testing.T) {
	for name, tc := range map[string]struct {
		in   string
		end  bool
		want string
	}{
		"date-only after is the start of the day": {"2026-11-18", false, "2026-11-18T00:00:00"},
		"date-only before is the end of the day":  {"2026-11-18", true, "2026-11-18T23:59:59"},
		"local time stays (after)":                {"2026-11-18T08:30:00", false, "2026-11-18T08:30:00"},
		"local time stays (before)":               {"2026-11-18T08:30:00", true, "2026-11-18T08:30:00"},
		"offset stays":                            {"2026-11-18T08:30:00+01:00", true, "2026-11-18T08:30:00+01:00"},
		"Z stays":                                 {"2026-11-18T08:30:00Z", false, "2026-11-18T08:30:00Z"},
		"empty stays empty":                       {"", true, ""},
	} {
		s := tc.in
		if _, err := resolveBound("x", &s, tc.end); err != nil || s != tc.want {
			t.Errorf("%s: got %q err %v, want %q", name, s, err, tc.want)
		}
	}
	if _, err := resolveBound("x", nil, true); err != nil {
		t.Errorf("nil bound is absent: %v", err)
	}
	bad := "2026-11-18 08:00"
	if _, err := resolveBound("x_before", &bad, true); err == nil || !strings.Contains(err.Error(), "x_before") || bad != "2026-11-18 08:00" {
		t.Errorf("bad date: want error naming the field and no rewrite, got %v %q", err, bad)
	}
	var none string
	if got, _ := resolveBound("x", &none, false); !got.IsZero() {
		t.Error("empty bound resolves to the zero time")
	}
	from, to := "2026-11-18", "2026-11-18"
	start, _ := resolveBound("x", &from, false)
	end, _ := resolveBound("x", &to, true)
	if got := end.t.Sub(start.t); got != 24*time.Hour-time.Second {
		t.Errorf("a day spans start..end of the same local day, got %s", got)
	}
}

func TestResolveRange(t *testing.T) {
	for name, tc := range map[string]struct {
		after, before string
		ok            bool
	}{
		"same date-only day covers the day":  {"2026-11-18", "2026-11-18", true},
		"after the day before":               {"2026-11-17", "2026-11-18", true},
		"after later day":                    {"2026-11-19", "2026-11-18", false},
		"after is the last second of before": {"2026-11-18T23:59:59", "2026-11-18", true},
		"one second later":                   {"2026-11-19T00:00:00", "2026-11-18", false},
		"timestamps in order":                {"2026-11-18T08:00:00", "2026-11-18T09:00:00", true},
		"timestamps reversed":                {"2026-11-18T09:00:00", "2026-11-18T08:00:00", false},
		"equal timestamps":                   {"2026-11-18T08:00:00", "2026-11-18T08:00:00", true},
		"offsets compared as instants":       {"2026-11-18T12:00:00Z", "2026-11-18T12:30:00+01:00", false},
		"offsets in order":                   {"2026-11-18T11:00:00Z", "2026-11-18T12:30:00+01:00", true},
		"only after":                         {"2030-01-01", "", true},
		"only before":                        {"", "2000-01-01", true},
	} {
		a, b := tc.after, tc.before
		err := resolveRange("due", &a, &b)
		if (err == nil) != tc.ok {
			t.Errorf("%s: ok=%v, err %v", name, tc.ok, err)
		}
		if err != nil && !tc.ok && !strings.Contains(err.Error(), "due_after") {
			t.Errorf("%s: error should name the fields: %v", name, err)
		}
	}
	after := "2026-11-18"
	if err := resolveRange("added", &after, nil); err != nil || after != "2026-11-18T00:00:00" {
		t.Errorf("nil before: %v %q", err, after)
	}
}

// dateWire is every date bound of list_tasks and the key it travels under.
var dateWire = []struct {
	key string
	set func(*TaskFilter, string)
	end bool // a _before bound
}{
	{"due_before", func(f *TaskFilter, s string) { f.DueBefore = s }, true},
	{"due_after", func(f *TaskFilter, s string) { f.DueAfter = s }, false},
	{"defer_before", func(f *TaskFilter, s string) { f.DeferBefore = s }, true},
	{"defer_after", func(f *TaskFilter, s string) { f.DeferAfter = s }, false},
	{"planned_before", func(f *TaskFilter, s string) { f.PlannedBefore = s }, true},
	{"planned_after", func(f *TaskFilter, s string) { f.PlannedAfter = s }, false},
	{"completed_before", func(f *TaskFilter, s string) { f.CompletedBefore = s }, true},
	{"completed_after", func(f *TaskFilter, s string) { f.CompletedAfter = s }, false},
	{"modified_after", func(f *TaskFilter, s string) { f.ModifiedAfter = s }, false},
	{"added_after", func(f *TaskFilter, s string) { f.AddedAfter = s }, false},
}

func TestListTasksSendsResolvedBounds(t *testing.T) {
	for _, w := range dateWire {
		clock := "T00:00:00"
		if w.end {
			clock = "T23:59:59"
		}
		for in, want := range map[string]string{"2026-11-18": "2026-11-18" + clock, "2026-11-18T08:30:00": "2026-11-18T08:30:00", "2026-11-18T08:30:00+01:00": "2026-11-18T08:30:00+01:00"} {
			r := &captureRunner{res: `[]`}
			var f TaskFilter
			w.set(&f, in)
			if _, err := (&Bridge{Runner: r}).ListTasks(context.Background(), f); err != nil {
				t.Fatalf("%s=%s: %v", w.key, in, err)
			}
			if !strings.Contains(r.src, `\"`+w.key+`\":\"`+want+`\"`) {
				t.Errorf("%s=%s: want %s on the wire: %s", w.key, in, want, r.src)
			}
		}
	}
}

func TestListTasksSendsFlagFilters(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		f    TaskFilter
		want string
	}{
		"untagged":        {TaskFilter{Untagged: &yes}, `\"untagged\":true`},
		"tagged":          {TaskFilter{Untagged: &no}, `\"untagged\":false`},
		"repeating":       {TaskFilter{Repeating: &yes}, `\"repeating\":true`},
		"not repeating":   {TaskFilter{Repeating: &no}, `\"repeating\":false`},
		"dropped":         {TaskFilter{Dropped: &yes}, `\"dropped\":true`},
		"not dropped":     {TaskFilter{Dropped: &no}, `\"dropped\":false`},
		"completed false": {TaskFilter{Completed: &no, DueAfter: "2026-11-18"}, `\"completed\":false`},
	} {
		r := &captureRunner{res: `[]`}
		if _, err := (&Bridge{Runner: r}).ListTasks(context.Background(), tc.f); err != nil || !strings.Contains(r.src, tc.want) {
			t.Errorf("%s: want %s, err %v, src %s", name, tc.want, err, r.src)
		}
	}
	r := &captureRunner{res: `[]`}
	if _, err := (&Bridge{Runner: r}).ListTasks(context.Background(), TaskFilter{}); err != nil {
		t.Fatal(err)
	}
	args := r.src[strings.LastIndex(r.src, "JSON.parse("):] // the script body also names every filter
	for _, k := range []string{"untagged", "repeating", "dropped", "_after", "_before"} {
		if strings.Contains(args, k) {
			t.Errorf("unset filter %s must not be sent: %s", k, args)
		}
	}
}

// The error shows the client's values, not the timestamps the bounds expand to.
func TestResolveRangeErrorShowsOriginalValues(t *testing.T) {
	f := TaskFilter{DueAfter: "2026-11-20", DueBefore: "2026-11-19"}
	if err := f.resolve(); err == nil || err.Error() != "due_after: 2026-11-20 is later than due_before 2026-11-19" {
		t.Fatalf("got %v", err)
	}
}

func TestListTasksRejectsContradictions(t *testing.T) {
	yes, no := true, false
	for name, tc := range map[string]struct {
		f    TaskFilter
		want string
	}{
		"after later than before":           {TaskFilter{DueAfter: "2026-11-20", DueBefore: "2026-11-19"}, "due_after"},
		"completed after later than before": {TaskFilter{CompletedAfter: "2026-11-20", CompletedBefore: "2026-11-19"}, "completed_after"},
		"completed false with after":        {TaskFilter{Completed: &no, CompletedAfter: "2026-11-01"}, "completed: false"},
		"completed false with before":       {TaskFilter{Completed: &no, CompletedBefore: "2026-11-01"}, "completed: false"},
		"dropped with completed":            {TaskFilter{Dropped: &yes, Completed: &yes}, "dropped: true"},
		"dropped with completed bound":      {TaskFilter{Dropped: &yes, CompletedAfter: "2026-11-01"}, "dropped: true"},
		"available with completed":          {TaskFilter{Available: &yes, Completed: &yes}, "available: true"},
		"available with completed bound":    {TaskFilter{Available: &yes, CompletedBefore: "2026-11-01"}, "available: true"},
		"available with dropped":            {TaskFilter{Available: &yes, Dropped: &yes}, "available: true"},
		"inbox with project":                {TaskFilter{Inbox: &yes, Project: "p"}, "inbox"},
	} {
		r := &captureRunner{res: `[]`}
		_, err := (&Bridge{Runner: r}).ListTasks(context.Background(), tc.f)
		if err == nil || !strings.Contains(err.Error(), tc.want) || r.src != "" {
			t.Errorf("%s: want error naming %q before any script, got %v src=%q", name, tc.want, err, r.src)
		}
	}
	for name, f := range map[string]TaskFilter{
		"completed with bound":          {Completed: &yes, CompletedAfter: "2026-11-01"},
		"completed bound alone":         {CompletedAfter: "2026-11-01", CompletedBefore: "2026-11-30"},
		"dropped with not completed":    {Dropped: &yes, Completed: &no},
		"available false with dropped":  {Available: &no, Dropped: &yes},
		"available with not dropped":    {Available: &yes, Dropped: &no},
		"available with not completed":  {Available: &yes, Completed: &no, DueBefore: "2026-11-30"},
		"same-day range":                {DueAfter: "2026-11-18", DueBefore: "2026-11-18"},
		"inbox false with project":      {Inbox: &no, Project: "p"},
		"modified and added both bound": {ModifiedAfter: "2026-11-01", AddedAfter: "2026-10-01"},
	} {
		if _, err := (&Bridge{Runner: &captureRunner{res: `[]`}}).ListTasks(context.Background(), f); err != nil {
			t.Errorf("%s: %v", name, err)
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
	if !strings.Contains(r.src, `\"review_due_before\":\"2026-12-02T23:59:59\"`) {
		t.Errorf("a date-only bound must cover its whole day on the wire: %s", r.src)
	}
}

func TestListProjectsRejectsBadReviewDate(t *testing.T) {
	r := &captureRunner{res: `[]`}
	if _, err := (&Bridge{Runner: r}).ListProjects(context.Background(), ProjectFilter{ReviewDueBefore: "soon"}); err == nil || r.src != "" {
		t.Fatalf("want validation error before any script, got %v src=%q", err, r.src)
	}
}

func TestProjectDecodesTypeEstimateRepetition(t *testing.T) {
	r := &captureRunner{res: `[{"id":"p1","name":"P","status":"Active","type":"SingleActions","completedByChildren":true,"estimatedMinutes":90,"repetition":` + repJSON + `,"tags":[]},{"id":"p2","name":"Q","status":"Active","type":"Parallel","completedByChildren":false,"tags":[]}]`}
	got, err := (&Bridge{Runner: r}).ListProjects(context.Background(), ProjectFilter{})
	if err != nil || len(got) != 2 {
		t.Fatalf("got %+v err %v", got, err)
	}
	a := got[0]
	if a.Type != "SingleActions" || !a.CompletedByChildren || a.EstimatedMinutes == nil || *a.EstimatedMinutes != 90 || a.Repetition == nil || a.Repetition.Anchor != "DueDate" || !a.Repetition.CatchUp {
		t.Fatalf("bad decode %+v", a)
	}
	if b := got[1]; b.Type != "Parallel" || b.CompletedByChildren || b.EstimatedMinutes != nil || b.Repetition != nil {
		t.Fatalf("want omitted fields %+v", b)
	}
	raw, err := json.Marshal(got[1])
	if err != nil || strings.Contains(string(raw), "sequential") || !strings.Contains(string(raw), `"type":"Parallel"`) || !strings.Contains(string(raw), `"completedByChildren":false`) {
		t.Fatalf("type and completedByChildren always present, no sequential: %s %v", raw, err)
	}
}

func TestProjectTypeCheck(t *testing.T) {
	for _, ok := range ProjectType("").names() {
		if err := checkEnum("type", ProjectType(ok)); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "parallel", "Single", "Sequential "} {
		if err := checkEnum("type", ProjectType(bad)); err == nil {
			t.Errorf("%q: want error", bad)
		}
	}
	if got := strings.Join(ProjectType("").names(), ","); got != "Parallel,Sequential,SingleActions" {
		t.Errorf("project types: %s", got)
	}
}
