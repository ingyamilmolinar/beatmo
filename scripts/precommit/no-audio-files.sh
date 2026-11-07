#!/usr/bin/env bash
# No audio file is ever committed: .gitignore hides audio, so this catches the rest (git add -f, files tracked before
# the rule). Deleting a tracked audio file is always allowed. Deliberately no skip switch.

set -euo pipefail

# Extensions are matched case-insensitively (sample libraries ship .WAV/.Wav).
audio_re='\.(wav|wave|mp3|aif|aiff|aifc|flac|ogg|oga|opus|m4a|aac|wma|caf)$'

# A = added, C = copied, M = modified, R = renamed-to. D is not listed: removing
# an audio file from the repository is exactly what we want.
offenders="$(git diff --cached --name-only --diff-filter=ACMR -z \
    | tr '\0' '\n' | grep -iE "${audio_re}" || true)"

if [ -n "${offenders}" ]; then
    cat >&2 <<EOF
pre-commit: audio files must never be committed:
$(printf '%s\n' "${offenders}" | sed 's/^/  - /')

Reference audio belongs in \$BEATMO_WAV_REFS (default ~/.beatmo/wav-refs);
register it with \`make refs-adopt\`. Unstage with:

    git rm --cached -- <file>

EOF
    exit 1
fi
