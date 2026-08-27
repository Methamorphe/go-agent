package bounded

import (
	"strings"
	"testing"
)

func TestPreviewKeepsHeadAndTailBounded(t *testing.T) {
	preview := NewPreview(10)
	_, _ = preview.Write([]byte("0123456789ABCDEFGHIJ"))
	got := preview.String()
	if !preview.Truncated() {
		t.Fatal("expected truncation")
	}
	if !strings.Contains(got, "01234") || !strings.Contains(got, "FGHIJ") {
		t.Fatalf("preview = %q", got)
	}
}
