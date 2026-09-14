#!/usr/bin/env python3
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location("verify_homebrew", Path(__file__).with_name("verify-homebrew.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FormulaVerificationTests(unittest.TestCase):
    def setUp(self):
        self.assets = {target: {"name": f"sag_1.2.3_{target}.tar.gz", "sha256": str(i) * 64}
                       for i, target in enumerate(("darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"), 1)}
        self.source = ""
        for platform, stanza in (("darwin", "on_macos"), ("linux", "on_linux")):
            self.source += f"  {stanza} do\n"
            if platform == "linux":
                self.source += '    depends_on "alsa-lib"\n'
            for arch in ("arm64", "amd64"):
                self.source += "    if Hardware::CPU.arm?\n" if arch == "arm64" else "    else\n"
                asset = self.assets[f"{platform}_{arch}"]
                self.source += f'      url "https://github.com/steipete/sag/releases/download/v1.2.3/{asset["name"]}"\n      sha256 "{asset["sha256"]}"\n'
            self.source += "    end\n  end\n"

    def test_accepts_platform_dependencies(self):
        module.verify("v1.2.3", self.assets, self.source)

    def test_rejects_wrong_target_hash_version_or_missing_branch(self):
        for source in (
            self.source.replace("darwin_arm64.tar.gz", "darwin_amd64.tar.gz"),
            self.source.replace("1" * 64, "f" * 64),
            self.source.replace("/v1.2.3/", "/v1.2.2/"),
            self.source.split("  on_linux do")[0],
            self.source + '  url "https://example.com/extra.tar.gz"\n',
        ):
            with self.subTest(source=source), self.assertRaises(ValueError):
                module.verify("v1.2.3", self.assets, source)


if __name__ == "__main__":
    unittest.main()
