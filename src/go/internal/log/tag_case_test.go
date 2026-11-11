package log

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNoUppercaseInfoTags scans production Go sources for Infof/Warnf/Errorf
// calls whose leading "[tag]" contains an uppercase letter. All bracket tags
// must be lowercase (the project-wide convention). Test files are skipped.
func TestNoUppercaseInfoTags(t *testing.T) {
	root := filepath.Join("..", "..") // src/go
	tagRE := regexp.MustCompile(`\.(?:Infof|Warnf|Errorf|Tracef|Debugf)\("\[([^\]]+)\]`)
	upper := regexp.MustCompile(`[A-Z]`)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range tagRE.FindAllSubmatch(b, -1) {
			tag := string(m[1])
			if upper.MatchString(tag) {
				t.Errorf("%s: uppercase tag [%s] — tags must be lowercase", path, tag)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
