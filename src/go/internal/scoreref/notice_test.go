package scoreref

import (
	"flag"
	"os"
	"testing"
)

var updateNotice = flag.Bool("update", false, "rewrite NOTICE-scores.md from the catalogue")

// Stale attribution is worse than none: it credits the wrong people for the files shipped.
func TestNoticeIsUpToDate(t *testing.T) {
	want, err := RenderNotice()
	if err != nil {
		t.Fatalf("RenderNotice: %v", err)
	}
	if *updateNotice {
		if err := os.WriteFile(NoticeFile, []byte(want), 0o644); err != nil {
			t.Fatalf("write %s: %v", NoticeFile, err)
		}
		t.Logf("rewrote %s", NoticeFile)
		return
	}
	got, err := os.ReadFile(NoticeFile)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with -update)", NoticeFile, err)
	}
	if string(got) != want {
		t.Errorf("%s is stale — regenerate with:\n"+
			"  go test ./internal/scoreref/ -run TestNoticeIsUpToDate -update", NoticeFile)
	}
}
