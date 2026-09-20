package config

import (
	"testing"

	"elebbs/internal/cfgrec"
)

func TestLanguageIndexIsOneBased(t *testing.T) {
	if LanguageIndex(0) != 0 || LanguageIndex(1) != 0 || LanguageIndex(3) != 2 {
		t.Fatalf("LanguageIndex 0/1/3 = %d %d %d", LanguageIndex(0), LanguageIndex(1), LanguageIndex(3))
	}
}

func TestLanguageRecSizeDetectsPackedRecord(t *testing.T) {
	if languageRecSize(466) != cfgrec.LanguageSize {
		t.Fatalf("466 -> %d", languageRecSize(466))
	}
	if languageRecSize(812) != cfgrec.LanguageSizeRA {
		t.Fatalf("2*406 -> %d", languageRecSize(812))
	}
	if languageRecSize(932) != cfgrec.LanguageSize {
		t.Fatalf("2*466 -> %d", languageRecSize(932))
	}
}
