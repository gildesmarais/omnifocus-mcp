package omni

import (
	"context"
	"errors"
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
	if err := (&Bridge{Runner: &fake{}}).Call(context.Background(), "nope", nil, new(any)); err == nil {
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
}
