#!/usr/bin/env bash
set -euo pipefail

sha=$(git rev-parse HEAD)

# A failed gate must explain itself: the release run otherwise shows only a
# non-zero exit while the cause lives in another workflow run.
watch_full_ci() {
  local run_id=$1
  if gh run watch "$run_id" --interval 10 --exit-status; then
    return 0
  fi
  {
    echo "### Full CI failed for $sha"
    echo "https://github.com/$GITHUB_REPOSITORY/actions/runs/$run_id"
    echo '```'
    gh run view "$run_id" --log-failed 2>/dev/null | tail -n 120 || true
    echo '```'
  } >> "$GITHUB_STEP_SUMMARY"
  return 1
}

# Push/PR smoke is not a release gate. Only full verification of this exact
# commit may supply release build inputs. A newer failed re-run must not hide an
# older green verification of the same commit, so reuse a successful run first
# and fall back to the newest one only to surface why verification is missing.
run_id=$(gh run list --workflow ci.yml --commit "$sha" --limit 30 \
  --json databaseId,event,displayTitle,conclusion \
  --jq '[.[] | select((.event == "schedule" or .event == "workflow_dispatch") and (.displayTitle | startswith("Full CI (")))]
    | (map(select(.conclusion == "success"))[0] // .[0]) | .databaseId // empty')
if [[ -n "$run_id" ]]; then
  watch_full_ci "$run_id"
  artifact=$(gh api "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/artifacts" \
    --jq '.artifacts[] | select(.name == "ci-distribution" and .expired == false) | .id')
  if [[ -z "$artifact" ]]; then run_id=''; fi
fi
if [[ -z "$run_id" ]]; then
  started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  gh workflow run ci.yml --ref "$RELEASE_TAG" -f suite=full
  for attempt in {1..12}; do
    run_id=$(gh run list --workflow ci.yml --commit "$sha" --event workflow_dispatch \
      --created ">=$started" --json databaseId,displayTitle \
      --jq '[.[] | select(.displayTitle | startswith("Full CI ("))][0].databaseId // empty')
    if [[ -n "$run_id" ]]; then break; fi
    sleep 5
  done
  test -n "$run_id"
  watch_full_ci "$run_id"
  # A green check without its build input cannot be used by packaging.
  gh api "repos/$GITHUB_REPOSITORY/actions/runs/$run_id/artifacts" \
    --jq '.artifacts[] | select(.name == "ci-distribution" and .expired == false) | .id' | grep -q .
fi
echo "run-id=$run_id" >> "$GITHUB_OUTPUT"
echo "sha=$sha" >> "$GITHUB_OUTPUT"
echo "Verified full CI: https://github.com/$GITHUB_REPOSITORY/actions/runs/$run_id" >> "$GITHUB_STEP_SUMMARY"
