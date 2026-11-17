# Reference WAVs — SUPERSEDED

**This directory and file are superseded.** Song-tier ("Tier-I") reference
provenance now lives entirely in `internal/wavref/catalogue.json`, and the
audio itself fetches into `$BEATMO_WAV_REFS` (never committed — full
recordings are tens of MB) via:

```
make fetch-refs TIER=song
```

Coverage — which templates have a song reference and which don't — is
reported by:

```
make refs-doctor
```

`scripts/fetch_song_refs.sh` (the tool that used to regenerate this file) has
been deleted; `musopen.org` started returning HTTP 403 behind Cloudflare, so
it no longer worked. This file is kept only as a pointer for anyone who finds
it via an old link — do not add new rows here.
