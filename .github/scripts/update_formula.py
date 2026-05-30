#!/usr/bin/env python3
"""Updates the Homebrew formula in homebrew-tap after each release.

Reads bottle sha256 values from .bottle_shasums (written by create_bottles.sh)
and writes a formula with a proper bottle do block so that `brew install`
pours the prebuilt binary and never triggers the Xcode check.
"""

import base64
import json
import os
import sys
import urllib.request

TAG = os.environ["TAG"]
TOKEN = os.environ["HOMEBREW_TAP_GITHUB_TOKEN"]
VERSION = TAG.lstrip("v")

FORMULA_NAME = "lazyssh"
REPO = "Bestigor89"
TAP_API = f"https://api.github.com/repos/{REPO}/homebrew-tap/contents/Formula/{FORMULA_NAME}.rb"
BOTTLE_ROOT_URL = f"https://github.com/{REPO}/{FORMULA_NAME}/releases/download/{TAG}"


# ── Helpers ────────────────────────────────────────────────────────────────────

def fetch(url, token=None, method="GET", data=None):
    req = urllib.request.Request(url, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Accept", "application/vnd.github+json")
    req.add_header("Content-Type", "application/json")
    if data:
        req.data = json.dumps(data).encode()
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())


def read_bottle_shasums():
    """Read sha256 values produced by create_bottles.sh from .bottle_shasums."""
    shasums_path = os.path.join(
        os.environ.get("GITHUB_WORKSPACE", "."), ".bottle_shasums"
    )
    if not os.path.exists(shasums_path):
        sys.exit(f"ERROR: {shasums_path} not found. "
                 "Run create_bottles.sh before this script.")
    result = {}
    with open(shasums_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            brew_tag, sha = line.split("=", 1)
            result[brew_tag.strip()] = sha.strip()
    return result


# ── Source archive checksums (from GoReleaser's checksums.txt) ─────────────────
# These are only used as a fallback if brew builds from source.
# With a valid bottle block they are never fetched at install time.

BASE = f"https://github.com/{REPO}/{FORMULA_NAME}/releases/download/{TAG}"
checksums_url = f"{BASE}/checksums.txt"
print(f"Fetching checksums from {checksums_url}")
with urllib.request.urlopen(checksums_url) as r:
    checksums_raw = r.read().decode()


def src_sha(platform):
    for line in checksums_raw.splitlines():
        if platform in line:
            return line.split()[0]
    raise ValueError(f"SHA not found for platform: {platform}")


darwin_arm64 = src_sha(f"{FORMULA_NAME}_{VERSION}_darwin_arm64.tar.gz")
darwin_amd64 = src_sha(f"{FORMULA_NAME}_{VERSION}_darwin_amd64.tar.gz")
linux_arm64  = src_sha(f"{FORMULA_NAME}_{VERSION}_linux_arm64.tar.gz")
linux_amd64  = src_sha(f"{FORMULA_NAME}_{VERSION}_linux_amd64.tar.gz")

# ── Bottle sha256 values ───────────────────────────────────────────────────────

bottle_sha = read_bottle_shasums()

required_tags = ["arm64_sequoia", "sequoia", "x86_64_linux", "arm64_linux"]
for tag in required_tags:
    if tag not in bottle_sha:
        sys.exit(f"ERROR: missing sha256 for bottle tag '{tag}' in .bottle_shasums")

# ── Formula content ────────────────────────────────────────────────────────────
#
# Design notes:
#   • The "bottle do" block makes brew pour the prebuilt binary; Xcode is never
#     checked when a matching bottle is available.
#   • cellar: :any_skip_relocation is correct for all pure-Go static binaries.
#   • The conditional url/sha256 block below is the *source* fallback used only
#     when brew builds from source (no bottle match or --build-from-source flag).
#   • depends_on "go" => :build is only evaluated during a source build.

formula = f"""\
# typed: false
# frozen_string_literal: true

class Lazyssh < Formula
  desc "Keyboard-driven SSH manager and dual-pane SFTP file browser for the terminal"
  homepage "https://github.com/{REPO}/{FORMULA_NAME}"
  version "{VERSION}"
  license "PolyForm-Noncommercial-1.0.0"

  # Prebuilt bottles — brew pours these and skips the Xcode check entirely.
  bottle do
    root_url "{BOTTLE_ROOT_URL}"
    sha256 cellar: :any_skip_relocation, arm64_sequoia: "{bottle_sha['arm64_sequoia']}"
    sha256 cellar: :any_skip_relocation, sequoia:       "{bottle_sha['sequoia']}"
    sha256 cellar: :any_skip_relocation, x86_64_linux:  "{bottle_sha['x86_64_linux']}"
    sha256 cellar: :any_skip_relocation, arm64_linux:   "{bottle_sha['arm64_linux']}"
  end

  # Source archives — used only when building from source (--build-from-source).
  if OS.mac? && Hardware::CPU.arm?
    url "{BASE}/{FORMULA_NAME}_{VERSION}_darwin_arm64.tar.gz"
    sha256 "{darwin_arm64}"
  elsif OS.mac?
    url "{BASE}/{FORMULA_NAME}_{VERSION}_darwin_amd64.tar.gz"
    sha256 "{darwin_amd64}"
  elsif Hardware::CPU.arm?
    url "{BASE}/{FORMULA_NAME}_{VERSION}_linux_arm64.tar.gz"
    sha256 "{linux_arm64}"
  else
    url "{BASE}/{FORMULA_NAME}_{VERSION}_linux_amd64.tar.gz"
    sha256 "{linux_amd64}"
  end

  def install
    bin.install "{FORMULA_NAME}"
  end

  test do
    assert_match version.to_s, shell_output("\#{{bin}}/{FORMULA_NAME} --version")
  end
end
"""

# ── Push formula to homebrew-tap ───────────────────────────────────────────────

print(f"Fetching current formula from homebrew-tap")
current = fetch(TAP_API, token=TOKEN)
file_sha = current["sha"]

encoded = base64.b64encode(formula.encode()).decode()
result = fetch(TAP_API, token=TOKEN, method="PUT", data={
    "message": f"Update {FORMULA_NAME} to {VERSION} (with bottles)",
    "content": encoded,
    "sha": file_sha,
})
print(f"Formula updated: {result['commit']['sha']}")
