#!/usr/bin/env bash
# The one required check: fails unless every job in $NEEDS (the workflow's
# toJSON(needs)) succeeded. A skipped or cancelled job is not a pass.
set -euo pipefail
python3 - <<'PY'
import json, os, sys
needs = json.loads(os.environ["NEEDS"])
bad = {k: v["result"] for k, v in needs.items() if v["result"] != "success"}
for k, v in sorted(needs.items()):
    print(f"{v['result']:>10}  {k}")
sys.exit(1 if bad else 0)
PY
