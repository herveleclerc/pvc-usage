package cmd

import (
	"bytes"
	"strings"
	"testing"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestNewCmdRoot(t *testing.T) {
	streams := genericclioptions.NewTestIOStreamsDiscard()
	cmd := NewCmdRoot(streams)

	if cmd.Use != "pvc-usage" {
		t.Errorf("expected command use 'pvc-usage', got %q", cmd.Use)
	}

	if cmd.Flag("unused-only") == nil {
		t.Error("expected flag 'unused-only' to be present")
	}

	if cmd.Flag("min-age") == nil {
		t.Error("expected flag 'min-age' to be present")
	}

	if cmd.Flag("all-namespaces") == nil {
		t.Error("expected flag 'all-namespaces' to be present")
	}
}

func TestVersionCmd(t *testing.T) {
	var outBuf, errBuf bytes.Buffer
	streams := genericclioptions.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &outBuf,
		ErrOut: &errBuf,
	}

	cmd := NewCmdVersion(streams)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("version cmd failed: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "kubectl-pvc-usage") {
		t.Errorf("expected version output to contain 'kubectl-pvc-usage', got: %s", out)
	}
}
