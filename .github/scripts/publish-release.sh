#!/usr/bin/env bash
set -euo pipefail

# Run inside the flat artifact directory after exact-commit CI and native builds.
# User downloads and signed desktop updates have separate immutable releases.
release_tag="${1:?release tag is required}"
source_sha="${2:?verified source commit is required}"
: "${GH_REPO:?release repository is required}"
[[ "$release_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
[[ "$source_sha" =~ ^[0-9a-f]{40}$ ]]
version="${release_tag#v}"
updates_tag="desktop-updates-$release_tag"

downloads=(
  "denova-desktop-$release_tag-macos-arm64.dmg"
  "denova-desktop-$release_tag-windows-x64-setup.exe"
)
for target in macos-arm64 macos-x64 linux-arm64 linux-x64 windows-x64; do
  extension=tar.gz
  if [[ "$target" == windows-x64 ]]; then extension=zip; fi
  downloads+=("denova-webui-$release_tag-$target.$extension")
done
updates=()
for target in darwin-arm64 windows-amd64; do
  archive="denovadesktop-$version-$target.tar.gz"
  manifest="update-$target.json"
  test -s "$archive"
  test -s "$manifest"
  jq -e --arg version "$version" \
    --arg url "https://github.com/$GH_REPO/releases/download/$updates_tag/$archive" \
    --argjson size "$(wc -c < "$archive" | tr -d ' ')" \
    '.version == $version and .url == $url and .size == $size and (.signature | type == "string" and length > 0)' \
    "$manifest" >/dev/null
  updates+=("$archive" "$manifest")
done
for file in "${downloads[@]}"; do test -s "$file"; done

checksum_command=(sha256sum)
if ! command -v sha256sum >/dev/null 2>&1; then checksum_command=(shasum -a 256); fi
"${checksum_command[@]}" "${downloads[@]}" > checksums.txt
downloads+=(checksums.txt)

# Verify the remote inventory and bytes, including resumed published releases.
# No glob upload can accidentally add an installer script or compatibility copy.
verify_assets() {
  local tag="$1" expected actual file sum
  shift
  expected="$(for file in "$@"; do
    sum="$("${checksum_command[@]}" "$file")"
    printf '%s\tsha256:%s\n' "$file" "${sum%% *}"
  done | LC_ALL=C sort)"
  actual="$(gh release view "$tag" --json assets --jq '.assets[] | [.name, .digest] | @tsv' | LC_ALL=C sort)"
  if [[ "$actual" != "$expected" ]]; then
    echo "Error: release $tag has unexpected assets or checksums" >&2
    return 1
  fi
}

prepare_release() {
  local tag="$1" title="$2" notes="$3" state
  shift 3
  if state="$(gh release view "$tag" --json isDraft --jq .isDraft 2>/dev/null)"; then
    if [[ "$state" == false ]]; then
      # Published bytes cannot be replaced, even while recovering a failed run.
      verify_assets "$tag" "$@"
      return
    fi
    gh release edit "$tag" --title "$title" --notes-file "$notes"
    gh release upload "$tag" "$@" --clobber
  else
    gh release create "$tag" "$@" --draft --target "$source_sha" --latest=false --title "$title" --notes-file "$notes"
  fi
  verify_assets "$tag" "$@"
}

cat > DESKTOP_UPDATE_NOTES.md <<EOF
Automatic desktop update files for Denova ${release_tag}. For manual installation, use https://github.com/${GH_REPO}/releases/tag/${release_tag}.

Denova ${release_tag} 的桌面自动更新文件。手动安装请使用 https://github.com/${GH_REPO}/releases/tag/${release_tag}。
EOF
prepare_release "$updates_tag" "Desktop updates $release_tag" DESKTOP_UPDATE_NOTES.md "${updates[@]}"
prepare_release "$release_tag" "$release_tag" RELEASE_NOTES.md "${downloads[@]}"

# Archives and manifests are complete before the official updater discovers them.
# Only the human-facing release becomes latest, keeping the WebUI updater intact.
gh release edit "$updates_tag" --draft=false --latest=false
gh release edit "$release_tag" --draft=false --latest
echo "Published $release_tag with ${#downloads[@]} download assets and a separate desktop update feed."
