#!/usr/bin/env bash
# check-doc-links.sh — find broken relative links in docs/ and README.md.
#
# Usage: scripts/check-doc-links.sh [path ...]
#   Default paths: docs README.md (relative to the repository root).
# Exit status: 0 when every relative link resolves, 1 otherwise.
#
# Checks Markdown links and images of the form [text](target) where target is
# a relative path (no scheme, not an absolute URL, not a bare #anchor). Anchors
# are stripped before the path is resolved. Targets inside fenced code blocks
# are ignored. Needs python3.
set -euo pipefail

cd "$(git rev-parse --show-toplevel 2>/dev/null || dirname "$0")/."

python3 - "$@" <<'PY'
import os
import re
import sys

roots = sys.argv[1:] or ["docs", "README.md"]
link_re = re.compile(r"!?\[[^\]]*\]\(\s*<?([^)\s>]+)>?(?:\s+\"[^\"]*\")?\s*\)")
skip_prefix = ("http://", "https://", "mailto:", "ftp://", "tel:", "#")

def md_files(root):
    if os.path.isfile(root):
        yield root
        return
    for d, _, files in os.walk(root):
        for f in sorted(files):
            if f.lower().endswith((".md", ".markdown")):
                yield os.path.join(d, f)

broken = []
checked = 0
for root in roots:
    for path in md_files(root):
        in_fence = False
        with open(path, encoding="utf-8", errors="replace") as fh:
            for lineno, line in enumerate(fh, 1):
                if line.lstrip().startswith(("```", "~~~")):
                    in_fence = not in_fence
                    continue
                if in_fence:
                    continue
                for target in link_re.findall(line):
                    if target.startswith(skip_prefix) or "://" in target:
                        continue
                    rel = target.split("#", 1)[0]
                    if not rel:
                        continue
                    checked += 1
                    resolved = os.path.normpath(os.path.join(os.path.dirname(path), rel))
                    if not os.path.exists(resolved):
                        broken.append((path, lineno, target))

for path, lineno, target in broken:
    print(f"{path}:{lineno}: broken link -> {target}")
print(f"checked {checked} relative links in {', '.join(roots)}; {len(broken)} broken")
sys.exit(1 if broken else 0)
PY
