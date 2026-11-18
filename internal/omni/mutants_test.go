package omni

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// Tests pinning behavior that mutation testing showed was unasserted.

func TestDefaultTimeoutValues(t *testing.T) {
	if DefaultTimeout != 30*time.Second || DefaultWriteTimeout != 120*time.Second {
		t.Fatalf("timeouts = %v, %v", DefaultTimeout, DefaultWriteTimeout)
	}
	if got := (&Bridge{}).timeoutFor(scriptTasks); got != 30*time.Second {
		t.Fatalf("read default = %v", got)
	}
	if got := (&Bridge{}).timeoutFor(scriptDrop); got != 120*time.Second {
		t.Fatalf("write default = %v", got)
	}
	if b := (&Bridge{Timeout: time.Second, WriteTimeout: 2 * time.Second}); b.timeoutFor(scriptTasks) != time.Second || b.timeoutFor(scriptDrop) != 2*time.Second {
		t.Fatal("explicit timeouts must win over defaults")
	}
}

func TestNewUsesOsascriptRunner(t *testing.T) {
	if _, ok := New().Runner.(OsascriptRunner); !ok {
		t.Fatalf("New().Runner = %T", New().Runner)
	}
}

// A cancelled context stops exec before osascript starts, so this reaches the
// error branch of Run without shelling out.
func TestOsascriptRunnerCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := OsascriptRunner{}.Run(ctx, "x")
	if out != "" || !errors.Is(err, context.Canceled) || err.Error() != "osascript: context canceled" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestClassifyEachMarkerAlone(t *testing.T) {
	cause := errors.New("exit 1")
	for stderr, want := range map[string]error{
		"error (-1743)":              ErrNotAuthorized,
		"Not authorized to send":     ErrNotAuthorized,
		"error (-600)":               ErrNotRunning,
		"Application isn't running.": ErrNotRunning,
	} {
		if got := classify(stderr, cause); !errors.Is(got, want) {
			t.Errorf("%q: got %v, want %v", stderr, got, want)
		}
	}
	got := classify(" boom \n", cause)
	if errors.Is(got, ErrNotAuthorized) || errors.Is(got, ErrNotRunning) || !errors.Is(got, cause) || !strings.HasSuffix(got.Error(), ": boom") {
		t.Fatalf("generic: %v", got)
	}
}

func TestResolveTagsIDWinsOverNameCollision(t *testing.T) {
	tags := []Tag{{ID: "x", Name: "a"}, {ID: "y", Name: "x"}}
	ids, err := resolveTags([]string{"x"}, tags)
	if err != nil || len(ids) != 1 || ids[0] != "x" {
		t.Fatalf("%v %v", ids, err)
	}
}

func TestPatchEstimateCheckedOnlyWhenSetAndNotNull(t *testing.T) {
	// Unset: the zero value is never validated, so another field carries the patch.
	if _, err := newPatchItem(decodeEdit(t, `{"id":"a","name":"n"}`), nil); err != nil {
		t.Fatalf("unset estimate: %v", err)
	}
	// Null clears; the carried value is ignored.
	if _, err := newPatchItem(TaskEdit{ID: "a", EstimatedMinutes: Nullable[int]{Set: true, Null: true, Value: -5}}, nil); err != nil {
		t.Fatalf("null estimate: %v", err)
	}
	if _, err := newPatchItem(TaskEdit{ID: "a", EstimatedMinutes: Nullable[int]{Set: true, Value: -5}}, nil); err == nil {
		t.Fatal("set negative estimate must fail")
	}
}

func TestEditTasksReadsTagsOnlyForNonEmptyTagEdits(t *testing.T) {
	ok := `[{"id":"a","ok":true,"task":{"id":"a","name":"N","status":"Available","tags":[]}}]`
	for name, js := range map[string]string{
		"empty edit": `{"id":"a","name":"N","tags":{}}`,
		"no edit":    `{"id":"a","name":"N"}`,
		"one add":    `{"id":"a","tags":{"add":["home"]}}`,
		"one remove": `{"id":"a","tags":{"remove":["home"]}}`,
	} {
		wantRead := strings.HasPrefix(name, "one")
		r := &route{tags: `[{"id":"t1","name":"home"}]`, res: ok}
		if _, err := (&Bridge{Runner: r}).EditTasks(context.Background(), []TaskEdit{decodeEdit(t, js)}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if read := slices.Contains(r.scripts, "tags"); read != wantRead {
			t.Errorf("%s: tags read = %v, want %v (%v)", name, read, wantRead, r.scripts)
		}
	}
}

func TestStructMoveAndReorderGuards(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"move parent unset":  {`{"kind":"tag","id":"a","move":{}}`, "parent is required"},
		"move empty parent":  {`{"kind":"tag","id":"a","move":{"parent":""}}`, "use null for the root"},
		"move own parent":    {`{"kind":"tag","id":"a","move":{"parent":"a"}}`, "own parent"},
		"move and reorder":   {`{"kind":"tag","id":"a","move":{"parent":"p"},"reorder":{"after":"b"}}`, "mutually exclusive"},
		"reorder neither":    {`{"kind":"tag","id":"a","reorder":{}}`, "exactly one"},
		"reorder both":       {`{"kind":"tag","id":"a","reorder":{"before":"b","after":"c"}}`, "exactly one"},
		"reorder own before": {`{"kind":"tag","id":"a","reorder":{"before":"a"}}`, "own sibling"},
		"reorder own after":  {`{"kind":"tag","id":"a","reorder":{"after":"a"}}`, "own sibling"},
	} {
		_, err := validEdit(decodeStructEdit(t, tc.in), testTags)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: want error containing %q, got %v", name, tc.want, err)
		}
	}
	for name, in := range map[string]string{
		"move to root":   `{"kind":"tag","id":"a","move":{"parent":null}}`,
		"move to parent": `{"kind":"tag","id":"a","move":{"parent":"p"}}`,
		"reorder before": `{"kind":"tag","id":"a","reorder":{"before":"b"}}`,
		"reorder after":  `{"kind":"tag","id":"a","reorder":{"after":"b"}}`,
	} {
		if _, err := validEdit(decodeStructEdit(t, in), testTags); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCheckStatus(t *testing.T) {
	if err := checkStatus(addStatuses, kindFolder, ""); err != nil {
		t.Errorf("empty status on a kind without statuses: %v", err)
	}
	if err := checkStatus(addStatuses, kindFolder, "Active"); err == nil || !strings.Contains(err.Error(), "not supported for folder") {
		t.Errorf("folder add status: %v", err)
	}
	if err := checkStatus(editStatuses, kindFolder, "Active"); err != nil {
		t.Errorf("folder edit Active: %v", err)
	}
	if err := checkStatus(addStatuses, kindTag, "OnHold"); err != nil {
		t.Errorf("tag OnHold: %v", err)
	}
	if err := checkStatus(addStatuses, kindTag, "Done"); err == nil || !strings.Contains(err.Error(), "not one of") {
		t.Errorf("tag Done: %v", err)
	}
}
