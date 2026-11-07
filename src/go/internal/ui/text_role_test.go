package ui

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

func parseTypographySizes(md string) map[string]float64 {
	out := map[string]float64{}
	roleRe := regexp.MustCompile(`^  ([a-z-]+):\s*$`)
	sizeRe := regexp.MustCompile(`^    fontSize:\s*([0-9.]+)px`)
	inTypo := false
	cur := ""
	for _, line := range regexp.MustCompile(`\r?\n`).Split(md, -1) {
		if line == "typography:" {
			inTypo = true
			continue
		}
		if !inTypo {
			continue
		}
		// A new top-level key (no indent, not a comment) ends the block.
		if len(line) > 0 && line[0] != ' ' && line[0] != '#' {
			break
		}
		if m := roleRe.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if m := sizeRe.FindStringSubmatch(line); m != nil && cur != "" {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				out[cur] = v
			}
		}
	}
	return out
}

// DESIGN.md is the source of truth: a role whose size diverges, or a title site hardcoding a px, means updating
// both together.
func TestTextRoleSizesMatchDesignMD(t *testing.T) {
	body, err := os.ReadFile(findDesignMD(t))
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	sizes := parseTypographySizes(string(body))
	want := []struct {
		key  string
		role TextRole
	}{
		{"panel-title", RolePanelTitle},
		{"section-header", RoleSectionHeader},
		{"body", RoleBody},
		{"caption", RoleCaption},
	}
	for _, w := range want {
		md, ok := sizes[w.key]
		if !ok {
			t.Errorf("DESIGN.md typography.%s has no fontSize", w.key)
			continue
		}
		if got := w.role.size(); got != md {
			t.Errorf("role %v size = %vpx, but DESIGN.md typography.%s = %vpx — keep them in sync", w.role, got, w.key, md)
		}
	}
}

// A distinct type scale in every build mode: panel title > section header > body > caption.
func TestTextRoleHierarchy(t *testing.T) {
	pt := StyledTextHeight(RolePanelTitle)
	sh := StyledTextHeight(RoleSectionHeader)
	bd := StyledTextHeight(RoleBody)
	cap := StyledTextHeight(RoleCaption)
	if !(pt > sh && sh > bd && bd > cap) {
		t.Fatalf("role heights must strictly decrease: panelTitle=%d sectionHeader=%d body=%d caption=%d", pt, sh, bd, cap)
	}
	if cap < 10 {
		t.Fatalf("caption height %d too small to read", cap)
	}
}

// Guards against a constant or zero fallback.
func TestStyledTextWidthScalesWithLength(t *testing.T) {
	short := StyledTextWidth("Hi", RoleBody)
	long := StyledTextWidth("Hello world", RoleBody)
	if long <= short {
		t.Fatalf("expected longer string wider: short=%d long=%d", short, long)
	}
}
