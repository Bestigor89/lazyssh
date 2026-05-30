#!/usr/bin/env python3
"""Updates the Homebrew formula in homebrew-tap after each release."""

import base64
import json
import os
import subprocess
import sys
import urllib.request

TAG = os.environ["TAG"]
TOKEN = os.environ["HOMEBREW_TAP_GITHUB_TOKEN"]
VERSION = TAG.lstrip("v")
BASE = f"https://github.com/Bestigor89/lazyssh/releases/download/{TAG}"
TAP_API = "https://api.github.com/repos/Bestigor89/homebrew-tap/contents/Formula/lazyssh.rb"


def fetch(url, token=None, method="GET", data=None):
    req = urllib.request.Request(url, method=method)
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    if data:
        req.data = json.dumps(data).encode()
    with urllib.request.urlopen(req) as r:
        return json.loads(r.read())


# Download checksums
checksums_url = f"{BASE}/checksums.txt"
with urllib.request.urlopen(checksums_url) as r:
    checksums = r.read().decode()

def sha(platform):
    for line in checksums.splitlines():
        if platform in line:
            return line.split()[0]
    raise ValueError(f"SHA not found for {platform}")

darwin_arm64 = sha("darwin_arm64.tar.gz")
darwin_amd64 = sha("darwin_amd64.tar.gz")
linux_arm64  = sha("linux_arm64.tar.gz")
linux_amd64  = sha("linux_amd64.tar.gz")

formula = f"""\
# typed: false
# frozen_string_literal: true

class Lazyssh < Formula
  desc "Keyboard-driven SSH manager and dual-pane SFTP file browser for the terminal"
  homepage "https://github.com/Bestigor89/lazyssh"
  version "{VERSION}"
  license "PolyForm-Noncommercial-1.0.0"

  if OS.mac? && Hardware::CPU.arm?
    url "{BASE}/lazyssh_{VERSION}_darwin_arm64.tar.gz"
    sha256 "{darwin_arm64}"
  elsif OS.mac?
    url "{BASE}/lazyssh_{VERSION}_darwin_amd64.tar.gz"
    sha256 "{darwin_amd64}"
  elsif Hardware::CPU.arm?
    url "{BASE}/lazyssh_{VERSION}_linux_arm64.tar.gz"
    sha256 "{linux_arm64}"
  else
    url "{BASE}/lazyssh_{VERSION}_linux_amd64.tar.gz"
    sha256 "{linux_amd64}"
  end

  def install
    bin.install "lazyssh"
  end

  test do
    assert_match version.to_s, shell_output("\#{{bin}}/lazyssh --version")
  end
end
"""

# Get current file SHA from homebrew-tap
current = fetch(TAP_API, token=TOKEN)
file_sha = current["sha"]

# Push updated formula
encoded = base64.b64encode(formula.encode()).decode()
result = fetch(TAP_API, token=TOKEN, method="PUT", data={
    "message": f"Update lazyssh to {VERSION}",
    "content": encoded,
    "sha": file_sha,
})
print(f"Formula updated: {result['commit']['sha']}")
