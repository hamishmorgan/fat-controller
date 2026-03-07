package cli

import (
	"bytes"
	"testing"
)

func TestWriteStructured_UnsupportedFormat(t *testing.T) {
	var buf bytes.Buffer
	err := writeStructured(&buf, "yaml", map[string]string{"k": "v"})
	if err == nil {
		t.Fatalf("expected error")
	}
}
