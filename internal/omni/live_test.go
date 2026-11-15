//go:build live

package omni

import (
	"context"
	"testing"
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
