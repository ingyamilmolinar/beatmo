package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

func TestStyledCacheKeyIncludesFontGen(t *testing.T) {
	ClearTextCacheForTest()
	a := StyledTextSprite("Hola", RoleBody)
	i18n.BumpFontGeneration()
	b := StyledTextSprite("Hola", RoleBody)
	if a == b {
		t.Fatal("styled cache served a stale sprite across a font-generation bump")
	}
}

func TestPlainTextCacheKeyIncludesFontGen(t *testing.T) {
	ClearTextCacheForTest()
	a := TextSprite("Hola")
	i18n.BumpFontGeneration()
	b := TextSprite("Hola")
	if a == b {
		t.Fatal("plain text cache served a stale sprite across a font-generation bump")
	}
}
