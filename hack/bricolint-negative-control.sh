#!/usr/bin/env bash
# Copyright (c) 2026 the go-widgets/desktop authors. All rights reserved.
#
# SPDX-License-Identifier: BSD-3-Clause
#
# Negative control for the bricolint hand-drawn-UI guard: prove the guard BITES.
#
# A guard that never fires is worthless: if bricolint silently passed no matter
# what, the "Guard against hand-drawn UI" step would be green even after a raw
# painter primitive crept back into a Draw method. This script closes that gap by
# exercising the guard against a KNOWN violation:
#
#   1. clean tree                       -> bricolint exits 0 (must pass)
#   2. inject a raw p.FillRect(...) into a real painter-using Draw method
#                                       -> bricolint exits non-zero (must bite)
#   3. remove the injection             -> bricolint exits 0 again
#
# The injected primitive is written into an EXISTING Draw method that already has
# `p painter.Painter`, `v.Bounds()` and `th` in scope, so it COMPILES — the
# non-zero exit in step 2 is genuinely bricolint's diagnostic, not a build error.
# The original file is restored on every exit path via a trap.

set -euo pipefail

# Never let a parent go.work turn the clean-tree vet into a false failure.
export GOWORK=off

# Resolve the bricolint binary: honour $BRICOLINT (CI sets it to the installed
# path), else fall back to $(go env GOPATH)/bin/bricolint.
BRICOLINT="${BRICOLINT:-$(go env GOPATH)/bin/bricolint}"

# Run from the module root regardless of the caller's CWD.
cd "$(dirname "$0")/.."

ANCHOR_FILE="render/galleryview.go"
# A real painter-using Draw method in this app (leading tab preserved exactly as
# it appears in the source, so index()/grep match the literal line).
ANCHOR='func (v *galleryView) Draw(p painter.Painter, th *toolkit.Theme) {'
# The raw drawing primitive we inject — exactly the kind of hand-drawn chrome the
# guard exists to forbid. Compiles in the anchor method's scope.
MARKER='p.FillRect(v.Bounds(), th.Surface) // bricolint-negative-control INJECTED'
BACKUP="$ANCHOR_FILE.bricolint.bak"
RESTORE=0

vet() { go vet -vettool="$BRICOLINT" ./...; }

# Restore the anchor file ONLY once a real backup has been taken (RESTORE=1) and
# it is non-empty. Without RESTORE, a stale/empty leftover .bak (or a failure
# before step 2's cp) could be moved over $ANCHOR_FILE and clobber it.
restore() {
  [ "$RESTORE" = 1 ] && [ -s "$BACKUP" ] && mv -f "$BACKUP" "$ANCHOR_FILE"
  rm -f "$BACKUP"
  return 0
}
trap restore EXIT

echo "== step 1: clean tree must pass (exit 0) =="
vet
echo "clean: PASS"

echo "== step 2: inject a raw painter primitive into $ANCHOR_FILE =="
cp "$ANCHOR_FILE" "$BACKUP"; RESTORE=1   # arm restore ONLY once the backup is real
# Robust literal insert: print each line, and immediately after the anchor line
# emit the marker (tab-indented). index() is a literal substring match, so the
# regex metacharacters in the anchor are harmless.
awk -v anchor="$ANCHOR" -v marker="$MARKER" '
  { print }
  index($0, anchor) { print "\t" marker }
' "$BACKUP" >"$ANCHOR_FILE"

if ! grep -qF "$MARKER" "$ANCHOR_FILE"; then
  echo "ERROR: anchor not found in $ANCHOR_FILE; could not inject the negative-control primitive" >&2
  exit 1
fi

echo "== step 3: bricolint must now FAIL (the guard bites) =="
if vet; then
  echo "ERROR: bricolint PASSED with an injected raw painter primitive -- the guard does NOT bite!" >&2
  exit 1
fi
echo "injected: bricolint failed as expected"

echo "== step 4: remove the injection, clean tree passes again =="
restore
trap - EXIT
vet
echo "restored: PASS"

echo "negative control OK: the bricolint guard bites."
