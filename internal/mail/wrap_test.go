package mail

import (
	"strings"
	"testing"
)

func TestWrapLinesSoftCRAndHardCR(t *testing.T) {
	raw := "Hello" + string(SoftCR) + "there world, this is a longer run that should wrap."
	lines := WrapLines(raw, 20)
	if len(lines) < 2 {
		t.Fatalf("expected wrap, got %#v", lines)
	}
	joined := strings.Join(lines, "")
	if strings.Contains(joined, string(SoftCR)) {
		t.Fatalf("soft CR leaked: %#v", lines)
	}
	if !strings.Contains(joined, "Hello") || !strings.Contains(joined, "there") {
		t.Fatalf("lost text: %#v", lines)
	}
	hard := WrapLines("one\ntwo\n\nthree", 79)
	if len(hard) != 4 || hard[0] != "one" || hard[1] != "two" || hard[2] != "" || hard[3] != "three" {
		t.Fatalf("hard CR: %#v", hard)
	}
}

func TestWrapLinesLongWord(t *testing.T) {
	s := strings.Repeat("a", 25)
	lines := WrapLines(s, 10)
	if len(lines) < 2 {
		t.Fatalf("long word: %#v", lines)
	}
	for _, ln := range lines {
		if len(ln) > 10 {
			t.Fatalf("overlong %q", ln)
		}
	}
}
