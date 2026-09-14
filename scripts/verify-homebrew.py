#!/usr/bin/env python3
"""Bind each native formula branch to the published inventory without executing Ruby."""
import json
import re
import sys
from pathlib import Path


def verify(tag, assets, source):
    expected_targets = {"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"}
    if set(assets) != expected_targets:
        raise ValueError("expected all four Homebrew targets")
    platform = None
    architecture = None
    seen = set()
    lines = source.splitlines()
    for index, line in enumerate(lines):
        if line == "  on_macos do":
            platform = "darwin"
        elif line == "  on_linux do":
            platform = "linux"
        elif line == "  end":
            platform = architecture = None
        elif line == "    if Hardware::CPU.arm?":
            architecture = "arm64"
        elif line == "    else":
            if architecture != "arm64":
                raise ValueError("unexpected CPU branch")
            architecture = "amd64"
        elif line == "    end":
            architecture = None
        elif re.match(r"\s*url\b", line):
            target = f"{platform}_{architecture}"
            if target not in expected_targets or target in seen:
                raise ValueError("unexpected or duplicate formula URL")
            asset = assets[target]
            expected_url = f'      url "https://github.com/steipete/sag/releases/download/{tag}/{asset["name"]}"'
            expected_sha = f'      sha256 "{asset["sha256"]}"'
            if line != expected_url or index + 1 >= len(lines) or lines[index + 1] != expected_sha:
                raise ValueError(f"{target} URL/checksum differs from verified release")
            seen.add(target)
    if seen != expected_targets:
        raise ValueError("formula is missing verified targets")


if __name__ == "__main__":
    tag, assets_file, formula_file = sys.argv[1:]
    verify(tag, json.loads(Path(assets_file).read_text()), Path(formula_file).read_text())
    print(f"Homebrew formula matches all four verified {tag} archives")
