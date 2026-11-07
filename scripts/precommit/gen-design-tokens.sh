#!/usr/bin/env bash
# Rejects the commit when a design_*.gen.go is stale against DESIGN.md. SKIP_DESIGN_TOKENS_HOOK=1 skips it, but the
# freshness test then fails in CI.

set -euo pipefail

if [ "${SKIP_DESIGN_TOKENS_HOOK:-0}" = "1" ]; then
    exit 0
fi

# Only run when DESIGN.md or generator-related files are staged.
if ! git diff --cached --name-only | grep -qE '^(DESIGN\.md|src/go/cmd/gen_design_tokens/|src/go/internal/ui/design_(tokens|components|profile)\.gen\.go)'; then
    exit 0
fi

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if ! command -v make >/dev/null 2>&1; then
    echo "pre-commit: make not found; skipping design-token check" >&2
    exit 0
fi

# Regenerate.
make gen-design-tokens >/dev/null

stale_files=()
for f in \
    src/go/internal/ui/design_tokens.gen.go \
    src/go/internal/ui/design_components.gen.go \
    src/go/internal/ui/design_profile.gen.go; do
    if ! git diff --quiet -- "$f"; then
        stale_files+=("$f")
    fi
done

if [ ${#stale_files[@]} -gt 0 ]; then
    cat <<EOF
pre-commit: generated design files are stale:
$(printf '  - %s\n' "${stale_files[@]}")
DESIGN.md changed but the files above were not regenerated.
The hook just refreshed them for you. Stage the changes and commit again:

    git add ${stale_files[*]}
    git commit ...

EOF
    exit 1
fi
