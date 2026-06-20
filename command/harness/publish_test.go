package harness

import (
	"bytes"
	"testing"
)

// bufWriter is a minimal command.Writer for tests.
type bufWriter struct{ bytes.Buffer }

func (b *bufWriter) Flush() error { return nil }

func TestGetPublishCmd_Flags(t *testing.T) {
	cmd := GetPublishCmd(func() Writer { return &bufWriter{} })
	if cmd.Use != "publish" {
		t.Errorf("Use = %q", cmd.Use)
	}
	// The producer-facing flags that survive: dist, registry, no-sync.
	for _, f := range []string{"dist", "registry", "no-sync"} {
		if cmd.Flags().Lookup(f) == nil {
			t.Errorf("missing --%s flag", f)
		}
	}
	// The flags whose data now comes from the plugin manifest must be GONE.
	for _, f := range []string{"coordinate", "plugin", "evaluates", "plain-http"} {
		if cmd.Flags().Lookup(f) != nil {
			t.Errorf("--%s should have been removed (data comes from the plugin manifest)", f)
		}
	}
}
