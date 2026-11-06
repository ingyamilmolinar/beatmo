# instrument_pins

Built-in templates reference the instrument documents they play by content hash
(`instrument_docs: {"<id>": {"ref": "sha256:…"}}`, written by `cmd/gen-showcase`).
A ref normally resolves to a SHIPPED document in `../instruments/`.

When a shipped instrument changes (or is removed) but a template must keep its
old sound, put the old document here as `<hex>.json` — the hex part of its
`instrument.ContentHash`. `make asset-pack` packs it into the embedded pack's
documents (never into the shipped list), so the template's ref still resolves.

Normally this directory holds only this README. Guards:
`TestBuildPacksPinnedDocuments` (a misnamed pin fails the build) and
`TestEveryTemplateRefResolves` (every template ref is in the pack).
