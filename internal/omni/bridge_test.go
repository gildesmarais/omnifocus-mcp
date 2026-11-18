package omni

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fake struct {
	got string
	res string
	err error
}

func (f *fake) Run(ctx context.Context, src string) (string, error) {
	f.got = src
	return f.res, f.err
}

func TestPerspectivesDecodes(t *testing.T) {
	f := &fake{res: `[{"id":null,"name":"Inbox","builtin":true},{"id":"abc","name":"Mine","builtin":false}]`}
	got, err := (&Bridge{Runner: f}).Perspectives(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != nil || *got[1].ID != "abc" || !got[0].Builtin {
		t.Fatalf("unexpected: %+v", got)
	}
	if !strings.HasPrefix(f.got, prelude) || !strings.Contains(f.got, "\nJSON.stringify((") {
		t.Fatalf("source not wrapped: %.60s", f.got)
	}
}

func TestComposeEmbedsArgsAsLiteralNotCode(t *testing.T) {
	evil := `"); deleteObject(x); ("` + "\u2028"
	src, err := compose("(function(a){return a})", map[string]string{"q": evil})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src, "\u2028") || !strings.Contains(src, `\"); deleteObject`) {
		t.Fatalf("argument escaped its literal: %s", src)
	}
	if !strings.Contains(src, "JSON.parse(") {
		t.Fatal("args must be parsed from a JSON literal")
	}
}

func TestComposeEscapesLineSeparators(t *testing.T) {
	src, err := compose("(function(a){return a})", map[string]string{"q": "a\u2028b\u2029c"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(src, "\u2028\u2029") || !strings.Contains(src, `\u2028`) || !strings.Contains(src, `\u2029`) {
		t.Fatalf("separators not escaped: %s", src)
	}
}

func TestRunnerErrorIsWrapped(t *testing.T) {
	_, err := (&Bridge{Runner: &fake{err: ErrNotRunning}}).Perspectives(context.Background())
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("got %v", err)
	}
}

func TestBadJSONResult(t *testing.T) {
	if _, err := (&Bridge{Runner: &fake{res: "nope"}}).Perspectives(context.Background()); err == nil {
		t.Fatal("want decode error")
	}
}

func TestUnknownScript(t *testing.T) {
	if err := (&Bridge{Runner: &fake{}}).call(context.Background(), script{name: "nope"}, nil, new(any)); err == nil {
		t.Fatal("want error")
	}
}

type slow struct{}

func (slow) Run(ctx context.Context, _ string) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func TestTimeoutApplied(t *testing.T) {
	_, err := (&Bridge{Runner: slow{}, Timeout: 10 * time.Millisecond}).Perspectives(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
}

func TestClassify(t *testing.T) {
	cause := errors.New("exit 1")
	if !errors.Is(classify("execution error: Not authorized to send Apple events to OmniFocus. (-1743)", cause), ErrNotAuthorized) {
		t.Error("auth")
	}
	if !errors.Is(classify("Application isn't running. (-600)", cause), ErrNotRunning) {
		t.Error("running")
	}
	if errors.Is(classify("boom", cause), ErrNotRunning) {
		t.Error("generic")
	}
	if errors.Is(classify("execution error: Can't get object. (-1728)", cause), ErrNotRunning) {
		t.Error("-1728 is a script error, not 'not running'")
	}
	// Each marker alone is enough.
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
	if !errors.Is(got, cause) || !strings.HasSuffix(got.Error(), ": boom") {
		t.Errorf("generic error must wrap the cause and carry trimmed stderr: %v", got)
	}
}

func TestMarshalEscapesLineSeparators(t *testing.T) {
	b, err := json.Marshal("a\u2028b\u2029c")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(string(b), "\u2028\u2029") || !strings.Contains(string(b), `\u2028`) || !strings.Contains(string(b), `\u2029`) {
		t.Fatalf("json.Marshal must escape U+2028/2029: %q", b)
	}
}

func TestWriteTimeoutLongerAndWarnsOfPartialApply(t *testing.T) {
	b := &Bridge{Runner: slow{}}
	writes := map[string]bool{"add_tasks": true, "edit_tasks": true, "add_structure": true, "edit_structure": true, "drop": true, "delete": true}
	for _, s := range allScripts {
		want := DefaultTimeout
		if writes[s.name] {
			want = DefaultWriteTimeout
		}
		if s.write != writes[s.name] {
			t.Fatalf("%s: write flag %v, want %v", s.name, s.write, writes[s.name])
		}
		if got := b.timeoutFor(s); got != want {
			t.Fatalf("%s: timeout %v, want %v", s.name, got, want)
		}
	}
	if DefaultWriteTimeout < 120*time.Second {
		t.Fatal("write timeout too short")
	}
	b = &Bridge{Runner: slow{}, WriteTimeout: 10 * time.Millisecond}
	_, err := b.AddTasks(context.Background(), []TaskAdd{{Name: "x"}})
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "partially applied") || !strings.Contains(err.Error(), "re-list") {
		t.Fatalf("got %v", err)
	}
	_, err = (&Bridge{Runner: slow{}, Timeout: 10 * time.Millisecond}).ListTasks(context.Background(), TaskFilter{})
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "partially") {
		t.Fatalf("reads must not claim partial apply: %v", err)
	}
}

func TestOsascriptSourceTravelsOnStdinNotArgv(t *testing.T) {
	cmd := osascriptCmd(context.Background(), "SECRET-USER-DATA")
	for _, a := range cmd.Args {
		if strings.Contains(a, "SECRET") {
			t.Fatalf("source leaked into argv: %q", a)
		}
	}
	got, err := io.ReadAll(cmd.Stdin)
	if err != nil || string(got) != "SECRET-USER-DATA" {
		t.Fatalf("stdin = %q %v", got, err)
	}
}

var allScripts = []script{
	scriptListPerspectives, scriptTasks, scriptProjects, scriptTags, scriptFolders,
	scriptAddTasks, scriptEditTasks, scriptAddStructure, scriptEditStructure, scriptDrop, scriptDelete,
}

func TestEveryScriptFileExists(t *testing.T) {
	names := map[string]bool{}
	for _, s := range allScripts {
		if names[s.name] {
			t.Errorf("duplicate script %q in allScripts", s.name)
		}
		names[s.name] = true
		if _, err := s.body(); err != nil {
			t.Errorf("%s: %v", s.name, err)
		}
	}
	entries, err := scripts.ReadDir("js")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]bool{}
	for _, e := range entries {
		n := strings.TrimSuffix(e.Name(), ".js")
		if n == "prelude" {
			continue
		}
		files[n] = true
		if !names[n] {
			t.Errorf("js/%s has no script value in allScripts", e.Name())
		}
	}
	for n := range names {
		if !files[n] {
			t.Errorf("script %q has no js file", n)
		}
	}
}

func TestDefaultTimeouts(t *testing.T) {
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

// A cancelled context stops exec before osascript starts, so the error path
// of Run is reachable without shelling out.
func TestOsascriptRunnerReportsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := OsascriptRunner{}.Run(ctx, "x")
	if out != "" || !errors.Is(err, context.Canceled) || err.Error() != "osascript: context canceled" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
