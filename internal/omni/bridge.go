// Package omni runs Omni Automation scripts inside OmniFocus through osascript.
package omni

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

//go:embed js/*.js
var scripts embed.FS

// prelude is prepended to every script; see js/prelude.js.
var prelude = mustRead("js/prelude.js")

func mustRead(name string) string {
	b, err := scripts.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// DefaultTimeout bounds one read call; DefaultWriteTimeout bounds one write
// call. Writes get longer because a killed osascript does not stop OmniFocus
// from finishing the batch.
const (
	DefaultTimeout      = 30 * time.Second
	DefaultWriteTimeout = 120 * time.Second
)

// script identifies one embedded js/<name>.js and owns whether it mutates OmniFocus.
type script struct {
	name  string
	write bool
}

var (
	scriptListPerspectives = script{name: "list_perspectives"}
	scriptTasks            = script{name: "tasks"}
	scriptProjects         = script{name: "projects"}
	scriptTags             = script{name: "tags"}
	scriptFolders          = script{name: "folders"}
	scriptAddTasks         = script{name: "add_tasks", write: true}
	scriptEditTasks        = script{name: "edit_tasks", write: true}
	scriptAddStructure     = script{name: "add_structure", write: true}
	scriptEditStructure    = script{name: "edit_structure", write: true}
	scriptDrop             = script{name: "drop", write: true}
	scriptDelete           = script{name: "delete", write: true}
)

// body returns the embedded source of the script.
func (s script) body() (string, error) {
	b, err := scripts.ReadFile("js/" + s.name + ".js")
	if err != nil {
		return "", fmt.Errorf("unknown script %q: %w", s.name, err)
	}
	return string(b), nil
}

// Runner executes a complete Omni Automation source in OmniFocus and returns
// what the script returned. It is the only seam: tests fake it, osascript implements it.
type Runner interface {
	Run(ctx context.Context, source string) (string, error)
}

var (
	// ErrNotRunning means OmniFocus is not running (or not reachable).
	ErrNotRunning = errors.New("OmniFocus is not running; start it and retry")
	// ErrNotAuthorized means macOS Automation permission for OmniFocus is missing.
	ErrNotAuthorized = errors.New("not authorized to control OmniFocus; allow it for this process under System Settings > Privacy & Security > Automation")
)

// Bridge calls named embedded scripts with JSON arguments and decodes JSON results.
type Bridge struct {
	Runner       Runner
	Timeout      time.Duration // reads; zero means DefaultTimeout
	WriteTimeout time.Duration // writes; zero means DefaultWriteTimeout
}

func (b *Bridge) timeoutFor(s script) time.Duration {
	t, def := b.Timeout, DefaultTimeout
	if s.write {
		t, def = b.WriteTimeout, DefaultWriteTimeout
	}
	if t == 0 {
		return def
	}
	return t
}

// New returns a Bridge backed by /usr/bin/osascript.
func New() *Bridge { return &Bridge{Runner: OsascriptRunner{}} }

// Call runs the script's js file with args and decodes the JSON result into out.
// The file must be a function expression taking one argument (args).
func (b *Bridge) Call(ctx context.Context, s script, args, out any) error {
	body, err := s.body()
	if err != nil {
		return err
	}
	src, err := compose(body, args)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, b.timeoutFor(s))
	defer cancel()
	res, err := b.Runner.Run(ctx, src)
	if err != nil {
		if s.write && ctx.Err() != nil {
			return fmt.Errorf("%s: %w (the batch may have partially applied in OmniFocus; re-list before retrying)", s.name, err)
		}
		return fmt.Errorf("%s: %w", s.name, err)
	}
	if err := json.Unmarshal([]byte(res), out); err != nil {
		return fmt.Errorf("%s: decode result: %w", s.name, err)
	}
	return nil
}

// compose builds the Omni Automation source. Arguments travel as a JSON string
// literal that the script parses; user values are never spliced into code.
func compose(body string, args any) (string, error) {
	if args == nil {
		args = struct{}{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("encode args: %w", err)
	}
	lit, err := json.Marshal(string(raw)) // JSON string literal is a valid JS string literal
	if err != nil {
		return "", err
	}
	return prelude + "\nJSON.stringify((" + body + ")(JSON.parse(" + string(lit) + ")))", nil
}

// OsascriptRunner is the real Runner.
type OsascriptRunner struct{}

// jxaWrapper reads the composed script from stdin, so household data never
// appears in argv (ps) and large batches do not hit ARG_MAX.
const jxaWrapper = `ObjC.import('Foundation');
function run() {
  var data = $.NSFileHandle.fileHandleWithStandardInput.readDataToEndOfFile;
  var src = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js;
  return Application('OmniFocus').evaluateJavascript(src);
}`

// osascriptCmd builds the command; the source travels on stdin only.
func osascriptCmd(ctx context.Context, source string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/usr/bin/osascript", "-l", "JavaScript", "-e", jxaWrapper)
	cmd.Stdin = strings.NewReader(source)
	return cmd
}

// Run implements Runner.
func (OsascriptRunner) Run(ctx context.Context, source string) (string, error) {
	cmd := osascriptCmd(ctx, source)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("osascript: %w", ctx.Err())
		}
		return "", classify(stderr.String(), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// classify maps osascript stderr to actionable errors.
func classify(stderr string, cause error) error {
	switch {
	case strings.Contains(stderr, "-1743") || strings.Contains(stderr, "Not authorized"):
		return ErrNotAuthorized
	case strings.Contains(stderr, "-600") || strings.Contains(stderr, "isn't running"):
		return ErrNotRunning
	}
	return fmt.Errorf("osascript: %w: %s", cause, strings.TrimSpace(stderr))
}
