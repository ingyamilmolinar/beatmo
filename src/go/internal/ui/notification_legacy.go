package ui

import (
	"regexp"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Keys whose English text an older build or English session may have persisted frozen, matched back so the
// history retranslates.
var legacyNotifKeys = []i18n.Key{
	i18n.KeyNotifUndo, i18n.KeyNotifRedo, // arg'd; matched before others is fine (distinct prefixes)
	i18n.KeyNotifRenamedInstrument, i18n.KeyNotifErrLoadJSON,
	i18n.KeyNotifRecordingFailed, i18n.KeyNotifSavingRecording, i18n.KeyNotifCannotStartRecording,
	i18n.KeyNotifRecordingSaveFailed, i18n.KeyNotifRecordingSaved,
	i18n.KeyNotifErrLoadWAV, i18n.KeyNotifSelectedWAV, i18n.KeyNotifUpdatedWAVInst, i18n.KeyNotifLoadedWAVInst,
	// no-arg keys (exact match)
	i18n.KeyNotifInvalidName, i18n.KeyNotifImported, i18n.KeyNotifImportCanceled,
	i18n.KeyNotifCannotConnectInvisible, i18n.KeyNotifOnlyPerpendicular,
	i18n.KeyNotifInstNameEmpty, i18n.KeyNotifInvalidBPM,
}

type legacyNotifMatcher struct {
	key      i18n.Key
	re       *regexp.Regexp
	undoRedo bool
}

var (
	legacyNotifMatchers []legacyNotifMatcher
	legacyNotifOnce     sync.Once
	legacyNotifVerb     = regexp.MustCompile(`%[sdv]`)
)

func buildLegacyNotifMatchers() {
	for _, k := range legacyNotifKeys {
		tmpl := i18n.EN(k)
		// Literal segments escaped, each verb a non-greedy capture.
		parts := legacyNotifVerb.Split(tmpl, -1)
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		pat := "^" + strings.Join(parts, "(.+?)") + "$"
		legacyNotifMatchers = append(legacyNotifMatchers, legacyNotifMatcher{
			key:      k,
			re:       regexp.MustCompile(pat),
			undoRedo: k == i18n.KeyNotifUndo || k == i18n.KeyNotifRedo,
		})
	}
}

// An undo/redo's captured action name maps back to its key ("@key") so it retranslates too; !ok shows verbatim.
func resolveLegacyNotif(text string) (i18n.Key, []string, bool) {
	legacyNotifOnce.Do(buildLegacyNotifMatchers)
	for _, m := range legacyNotifMatchers {
		sub := m.re.FindStringSubmatch(text)
		if sub == nil {
			continue
		}
		args := append([]string(nil), sub[1:]...)
		if m.undoRedo && len(args) == 1 {
			if ak, ok := undoActionLabelKeys[args[0]]; ok {
				args[0] = "@" + string(ak)
			}
		}
		return m.key, args, true
	}
	return "", nil, false
}
