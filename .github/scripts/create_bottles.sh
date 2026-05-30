#!/usr/bin/env bash
# Creates Homebrew bottle tarballs for each platform and uploads them to the
# existing GitHub release. Run after GoReleaser has already created the release
# and uploaded the source archives.
#
# Required env:
#   TAG             — e.g. "v1.0.2"
#   GITHUB_TOKEN    — token with contents:write on Bestigor89/lazyssh
#
# Required tools: gh, curl, shasum (macOS) or sha256sum (Linux)

set -euo pipefail

TAG="${TAG:?TAG env var required}"
VERSION="${TAG#v}"   # strip leading "v" → 1.0.2
REPO="Bestigor89/lazyssh"
FORMULA_NAME="lazyssh"
RELEASE_DOWNLOAD="https://github.com/${REPO}/releases/download/${TAG}"

# sha256 wrapper — works on both macOS and Linux
sha256() {
  if command -v sha256sum &>/dev/null; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# platform_map: "<goreleaser_archive_suffix>|<brew_tag>|<brew_os>"
declare -a PLATFORMS=(
  "darwin_arm64|arm64_sequoia|darwin"
  "darwin_amd64|sequoia|darwin"
  "linux_amd64|x86_64_linux|linux"
  "linux_arm64|arm64_linux|linux"
)

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

SHA256_MAP=""   # accumulated newline-separated "brew_tag=sha256"

for entry in "${PLATFORMS[@]}"; do
  IFS='|' read -r goreleaser_suffix brew_tag brew_os <<< "$entry"

  archive_name="${FORMULA_NAME}_${VERSION}_${goreleaser_suffix}.tar.gz"
  archive_url="${RELEASE_DOWNLOAD}/${archive_name}"

  echo "==> Downloading ${archive_name}"
  curl -fsSL --retry 3 -o "${WORKDIR}/${archive_name}" "${archive_url}"

  # Extract the binary from the GoReleaser archive
  extract_dir="${WORKDIR}/extracted_${brew_tag}"
  mkdir -p "${extract_dir}"
  tar -xzf "${WORKDIR}/${archive_name}" -C "${extract_dir}" "${FORMULA_NAME}"

  # Build the bottle directory tree: lazyssh/1.0.2/bin/lazyssh
  #                                   lazyssh/1.0.2/.brew/lazyssh.rb
  bottle_root="${WORKDIR}/bottle_${brew_tag}"
  bin_dir="${bottle_root}/${FORMULA_NAME}/${VERSION}/bin"
  brew_dir="${bottle_root}/${FORMULA_NAME}/${VERSION}/.brew"
  mkdir -p "${bin_dir}" "${brew_dir}"

  cp "${extract_dir}/${FORMULA_NAME}" "${bin_dir}/${FORMULA_NAME}"
  chmod 755 "${bin_dir}/${FORMULA_NAME}"

  # The .brew/lazyssh.rb file is a placeholder formula for the bottle.
  # brew pours it successfully as long as the file is present; the canonical
  # copy lives in the tap repo and is authoritative at install time.
  cat > "${brew_dir}/${FORMULA_NAME}.rb" <<RUBY
# typed: false
# frozen_string_literal: true

class Lazyssh < Formula
  desc "Keyboard-driven SSH manager and dual-pane SFTP file browser for the terminal"
  homepage "https://github.com/Bestigor89/lazyssh"
  version "${VERSION}"
  license "PolyForm-Noncommercial-1.0.0"

  bottle do
    root_url "https://github.com/Bestigor89/lazyssh/releases/download/${TAG}"
  end

  def install
    bin.install "${FORMULA_NAME}"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/${FORMULA_NAME} --version")
  end
end
RUBY

  # Create the bottle tarball (single-dash filename = the asset name on the release)
  bottle_filename="${FORMULA_NAME}-${VERSION}.${brew_tag}.bottle.tar.gz"
  echo "==> Packing ${bottle_filename}"
  tar -czf "${WORKDIR}/${bottle_filename}" \
    -C "${bottle_root}" \
    "${FORMULA_NAME}/"

  checksum="$(sha256 "${WORKDIR}/${bottle_filename}")"
  echo "    sha256: ${checksum}"
  SHA256_MAP="${SHA256_MAP}${brew_tag}=${checksum}"$'\n'

  echo "==> Uploading ${bottle_filename} to GitHub release ${TAG}"
  gh release upload "${TAG}" "${WORKDIR}/${bottle_filename}" \
    --repo "${REPO}" \
    --clobber
done

# Write sha256 values to a file so the next step (update_formula.py) can read them
# without re-downloading the bottles.
SHASUMS_FILE="${GITHUB_WORKSPACE:-.}/.bottle_shasums"
printf '%s' "${SHA256_MAP}" > "${SHASUMS_FILE}"
echo "==> Bottle sha256 sums written to ${SHASUMS_FILE}"
cat "${SHASUMS_FILE}"
