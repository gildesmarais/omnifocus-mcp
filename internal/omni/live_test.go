//go:build live

package omni

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// Read-only: requires a running OmniFocus with Automation permission.
func TestLivePerspectives(t *testing.T) {
	ps, err := New().Perspectives(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 {
		t.Fatal("expected at least the built-in perspectives")
	}
	t.Logf("%d perspectives, first: %+v", len(ps), ps[0])
}

// Counts only: no names are logged.
func TestLiveReads(t *testing.T) {
	ctx := context.Background()
	b := New()
	yes := true
	timed := func(name string, f func() int) {
		t0 := time.Now()
		n := f()
		t.Logf("%-22s count=%d wall=%s", name, n, time.Since(t0).Round(time.Millisecond))
	}
	var all []Task
	timed("list_tasks inbox", func() int {
		ts, err := b.ListTasks(ctx, TaskFilter{Inbox: &yes})
		if err != nil {
			t.Fatal(err)
		}
		return len(ts)
	})
	timed("list_tasks available", func() int {
		ts, err := b.ListTasks(ctx, TaskFilter{Available: &yes})
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range ts {
			if !x.Available || x.Completed || x.Dropped {
				t.Fatalf("task %s not available", x.ID)
			}
		}
		return len(ts)
	})
	timed("list_tasks all", func() int {
		var err error
		if all, err = b.ListTasks(ctx, TaskFilter{}); err != nil {
			t.Fatal(err)
		}
		return len(all)
	})
	timed("list_tasks default 50", func() int {
		f := 50
		ts, err := b.ListTasks(ctx, TaskFilter{Limit: &f})
		if err != nil || len(ts) > 50 {
			t.Fatalf("%d %v", len(ts), err)
		}
		return len(ts)
	})
	for _, x := range all {
		if x.EstimatedMinutes != nil && *x.EstimatedMinutes <= 0 {
			t.Fatalf("task %s estimatedMinutes %d", x.ID, *x.EstimatedMinutes)
		}
		if x.Repetition != nil && (!strings.HasPrefix(x.Repetition.Rule, "FREQ=") || !slices.Contains(repetitionScheduleTypes, x.Repetition.ScheduleType) || !slices.Contains(repetitionAnchors, x.Repetition.Anchor)) {
			t.Fatalf("task %s repetition %+v", x.ID, *x.Repetition)
		}
		if x.Added == "" || x.Modified == "" {
			t.Fatalf("task %s lacks added/modified: %+v", x.ID, x)
		}
	}
	if len(all) > 0 {
		x, err := b.GetTask(ctx, all[0].ID)
		if err != nil || x.ID != all[0].ID || x.Name != all[0].Name {
			t.Fatalf("get_task round trip: %+v %v", x, err)
		}
		t.Log("get_task round trip ok")
	}
	var ps []Project
	timed("list_projects", func() int {
		var err error
		if ps, err = b.ListProjects(ctx, ProjectFilter{}); err != nil {
			t.Fatal(err)
		}
		return len(ps)
	})
	if len(ps) > 0 {
		if p, err := b.GetProject(ctx, ps[0].ID); err != nil || p.ID != ps[0].ID {
			t.Fatalf("get_project %v", err)
		}
	}
	timed("list_projects Active", func() int {
		a, err := b.ListProjects(ctx, ProjectFilter{Status: "Active"})
		if err != nil {
			t.Fatal(err)
		}
		return len(a)
	})
	var tags []Tag
	timed("list_tags", func() int {
		var err error
		if tags, err = b.ListTags(ctx); err != nil {
			t.Fatal(err)
		}
		return len(tags)
	})
	if len(tags) > 0 {
		if g, err := b.GetTag(ctx, tags[0].ID); err != nil || g.ID != tags[0].ID {
			t.Fatalf("get_tag %v", err)
		}
	}
	var fs []Folder
	timed("list_folders", func() int {
		var err error
		if fs, err = b.ListFolders(ctx); err != nil {
			t.Fatal(err)
		}
		return len(fs)
	})
	if len(fs) > 0 {
		if f, err := b.GetFolder(ctx, fs[0].ID); err != nil || f.ID != fs[0].ID {
			t.Fatalf("get_folder %v", err)
		}
	}
	if _, err := b.GetTask(ctx, "does-not-exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// Read-only. Checks every list_tasks filter against an oracle computed in Go
// from the unfiltered list, so the script's bound and flag logic is pinned on
// the real database. Pivots come from existing data; a field with no dates is skipped.
func TestLiveTaskFilters(t *testing.T) {
	ctx := context.Background()
	b := New()
	all, err := b.ListTasks(ctx, TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ids := func(ts []Task) []string {
		out := make([]string, len(ts))
		for i, x := range ts {
			out[i] = x.ID
		}
		return out
	}
	// check lists with f and requires exactly the tasks of all satisfying keep, in order.
	check := func(name string, f TaskFilter, keep func(Task) bool) []Task {
		t.Helper()
		got, err := b.ListTasks(ctx, f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var want []Task
		for _, x := range all {
			if keep(x) {
				want = append(want, x)
			}
		}
		if !slices.Equal(ids(got), ids(want)) {
			t.Errorf("%s: got %d tasks, want %d", name, len(got), len(want))
		}
		t.Logf("%-28s count=%d", name, len(got))
		return got
	}

	yes, no := true, false
	check("untagged", TaskFilter{Untagged: &yes}, func(x Task) bool { return len(x.Tags) == 0 })
	check("tagged", TaskFilter{Untagged: &no}, func(x Task) bool { return len(x.Tags) > 0 })
	check("repeating", TaskFilter{Repeating: &yes}, func(x Task) bool { return x.Repetition != nil })
	check("not repeating", TaskFilter{Repeating: &no}, func(x Task) bool { return x.Repetition == nil })
	check("dropped", TaskFilter{Dropped: &yes}, func(x Task) bool { return x.Dropped })
	check("not dropped", TaskFilter{Dropped: &no}, func(x Task) bool { return !x.Dropped })
	check("completed", TaskFilter{Completed: &yes}, func(x Task) bool { return x.Completed })
	check("inbox", TaskFilter{Inbox: &yes}, func(x Task) bool { return x.Inbox })
	check("not inbox", TaskFilter{Inbox: &no}, func(x Task) bool { return !x.Inbox })
	check("available", TaskFilter{Available: &yes}, func(x Task) bool { return x.Available })
	check("not available", TaskFilter{Available: &no}, func(x Task) bool { return !x.Available })
	for _, x := range all {
		if x.Completed && x.Dropped {
			t.Fatalf("task %s is completed and dropped; the contradiction check assumes they are disjoint", x.ID)
		}
	}

	fields := []struct {
		name string
		get  func(Task) string
	}{
		{"due", func(x Task) string { return x.Due }},
		{"defer", func(x Task) string { return x.Defer }},
		{"planned", func(x Task) string { return x.Planned }},
		{"completed", func(x Task) string { return x.CompletedAt }},
		{"modified", func(x Task) string { return x.Modified }},
		{"added", func(x Task) string { return x.Added }},
	}
	const startOfDay, endOfDay = "T00:00:00", "T23:59:59"
	for _, fld := range fields {
		var vals []string
		for _, x := range all {
			if v := fld.get(x); v != "" {
				vals = append(vals, v)
			}
		}
		if len(vals) == 0 {
			t.Logf("%s: no task has this date, skipped", fld.name)
			continue
		}
		slices.Sort(vals)
		pivot := vals[len(vals)/2]
		day := pivot[:10]
		for _, w := range dateWire {
			if !strings.HasPrefix(w.key, fld.name+"_") {
				continue
			}
			for _, bound := range []string{pivot, day} {
				lo, hi := bound, bound
				if len(bound) == 10 {
					lo, hi = bound+startOfDay, bound+endOfDay
				}
				var f TaskFilter
				w.set(&f, bound)
				keep := func(x Task) bool {
					v := fld.get(x)
					if w.end {
						return v != "" && v <= hi
					}
					return v != "" && v >= lo
				}
				got := check(w.key+"="+bound, f, keep)
				if len(got) == 0 {
					t.Errorf("%s=%s: the pivot task itself must match (bounds are inclusive)", w.key, bound)
				}
				for _, x := range got {
					if fld.get(x) == "" {
						t.Errorf("%s=%s: task %s has no such date and must not match", w.key, bound, x.ID)
					}
				}
			}
		}
	}

	// The completed_* bounds find completed tasks with completed unset, and only those.
	if latest := slices.MaxFunc(all, func(a, b Task) int { return strings.Compare(a.CompletedAt, b.CompletedAt) }); latest.CompletedAt != "" {
		got := check("completed_after=latest", TaskFilter{CompletedAfter: latest.CompletedAt}, func(x Task) bool { return x.CompletedAt >= latest.CompletedAt && x.CompletedAt != "" })
		for _, x := range got {
			if !x.Completed || x.Dropped {
				t.Errorf("task %s from a completed bound is not completed", x.ID)
			}
		}
	}
	week := time.Now().AddDate(0, 0, -7).Format("2006-01-02T15:04:05")
	for _, x := range check("completed_after=7d ago", TaskFilter{CompletedAfter: week}, func(x Task) bool { return x.CompletedAt != "" && x.CompletedAt >= week }) {
		if x.CompletedAt < week {
			t.Errorf("task %s completed %s, before %s", x.ID, x.CompletedAt, week)
		}
	}

	// review_due_before follows the same rule on projects: a date-only value covers its whole day.
	ps, err := b.ListProjects(ctx, ProjectFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		if p.NextReview == "" {
			continue
		}
		day := p.NextReview[:10]
		prev, _ := time.Parse("2006-01-02", day)
		for _, d := range []string{day, prev.AddDate(0, 0, -1).Format("2006-01-02")} {
			got, err := b.ListProjects(ctx, ProjectFilter{ReviewDueBefore: d})
			if err != nil {
				t.Fatal(err)
			}
			var want []string
			for _, q := range ps {
				if q.NextReview != "" && q.NextReview <= d+endOfDay {
					want = append(want, q.ID)
				}
			}
			var have []string
			for _, q := range got {
				have = append(have, q.ID)
			}
			if !slices.Equal(have, want) {
				t.Errorf("review_due_before=%s: got %d projects, want %d", d, len(have), len(want))
			}
			if d == day && !slices.Contains(have, p.ID) {
				t.Errorf("review_due_before=%s must include the project due that day (%s)", d, p.NextReview)
			}
			if d != day && slices.Contains(have, p.ID) {
				t.Errorf("review_due_before=%s must exclude the project due the next day", d)
			}
		}
		break
	}
}
