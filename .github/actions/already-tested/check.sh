#!/usr/bin/env bash
# already-tested: decide whether a push needs its checks run again.
#
# A release passes through CI three times on the same code: on the pull
# request, on the push of its merge commit to main, and on the tag pushed at
# that merge commit. Only the first run tells us anything. This script prints
# tested=true when the pushed commit is a merge whose second parent (the PR
# head) already passed every named check, and main brought nothing in that can
# affect them — only paths matching IGNORE (or not matching RELEVANT) differ.
#
# Inputs (environment):
#   REPO      owner/name
#   SHA       the pushed commit
#   EVENT     github.event_name; anything but "push" prints tested=false
#   CHECKS    newline-separated check-run names that must have succeeded
#   RELEVANT  ERE of paths that can affect the checks (default: every path)
#   IGNORE    ERE of paths that cannot (default: none)
# Output: "tested=true|false" on stdout, reasons on stderr.
set -euo pipefail

say() { echo "already-tested: $*" >&2; }
answer() { echo "tested=$1"; exit 0; }

[[ "${EVENT:-}" == "push" ]] || { say "event ${EVENT:-none} always runs its checks"; answer false; }

parents=$(gh api "repos/${REPO}/commits/${SHA}" --jq '[.parents[].sha] | join(" ")')
read -r -a parentList <<< "$parents"
if (( ${#parentList[@]} != 2 )); then
    say "${SHA} is not a merge commit"; answer false
fi
prHead=${parentList[1]}

# Files main changed relative to the PR head. A file outside RELEVANT, or
# inside IGNORE, cannot change what the checks saw.
changed=$(gh api "repos/${REPO}/compare/${prHead}...${SHA}" --jq '.files[].filename')
mattering=$(printf '%s\n' "$changed" | grep -E "${RELEVANT:-.}" | grep -Ev "${IGNORE:-^$}" || true)
if [[ -n "$mattering" ]]; then
    say "main differs from the PR head in: $(echo "$mattering" | head -5 | paste -sd' ')"
    answer false
fi

while IFS= read -r check; do
    [[ -z "$check" ]] && continue
    ok=$(gh api -X GET "repos/${REPO}/commits/${prHead}/check-runs" -f check_name="$check" \
        --jq '[.check_runs[] | select(.conclusion == "success" and .app.slug == "github-actions")] | length')
    if [[ "$ok" == "0" ]]; then
        say "\"${check}\" has no successful run on the PR head ${prHead}"
        answer false
    fi
done <<< "${CHECKS}"

say "the PR head ${prHead} passed these checks on the same code; skipping them"
answer true
