import hashlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class Packaging(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="synology-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / "repo"
        shutil.copytree(ROOT, self.repo, ignore=shutil.ignore_patterns(".git", "dist", "__pycache__"))
        self.inputs = self.root / "inputs"
        self.inputs.mkdir()
        (self.inputs / "latest").write_text("4.2.7\n")
        for name, bits, machine in [("arm", 1, 40), ("amd64", 2, 62)]:
            header = bytearray(64)
            header[:7] = b"\x7fELF" + bytes([bits, 1, 1])
            header[16:20] = bytes([2, 0, machine, 0])
            (self.inputs / ("synology." + name)).write_bytes(header)
        self.env = dict(os.environ, DDNSTO_PATH=str(self.inputs), OUT_DIR=str(self.root / "out"))

    def run_build(self):
        return subprocess.run(["sh", str(self.repo / "build.sh")], cwd=self.root,
                              env=self.env, capture_output=True, text=True)

    def test_missing_binary_fails_without_spk(self):
        (self.inputs / "synology.arm").unlink()
        result = self.run_build()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(list(self.root.rglob("*.spk")))

    def test_single_package_missing_binary_fails(self):
        result = subprocess.run(["sh", "pkg.sh", "ddnsto_x86", "dsm7"], cwd=self.repo,
                                env=dict(self.env, X86_BIN=str(self.root / "missing")),
                                capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(list(self.repo.rglob("*.spk")))

    def test_wrong_architecture_fails(self):
        shutil.copyfile(self.inputs / "synology.arm", self.inputs / "synology.amd64")
        self.assertNotEqual(self.run_build().returncode, 0)
        self.assertFalse(list(self.root.rglob("*.spk")))

    def test_four_packages_from_any_directory_without_source_mutation(self):
        original = (self.repo / "arm/INFO-dsm6").read_bytes()
        result = self.run_build()
        self.assertEqual(result.returncode, 0, result.stderr)
        packages = sorted((self.root / "out").rglob("*.spk"))
        self.assertEqual(len(packages), 4)
        for package in packages:
            with tarfile.open(package) as outer:
                info = outer.extractfile("INFO").read()
                self.assertIn(b'version="4.2.7-1"', info)
                with tarfile.open(fileobj=io.BytesIO(outer.extractfile("package.tgz").read()), mode="r:gz") as inner:
                    arch = "arm" if "_arm-" in package.name else "amd64"
                    self.assertEqual(inner.extractfile("bin/ddnsto").read(), (self.inputs / ("synology." + arch)).read_bytes())
                    self.assertEqual(inner.getmember("bin/ddnsto").mode, 0o755)
        self.assertEqual((self.repo / "arm/INFO-dsm6").read_bytes(), original)
        sums = next((self.root / "out").rglob("SHA256SUMS")).read_text()
        for package in packages:
            self.assertIn(hashlib.sha256(package.read_bytes()).hexdigest(), sums)

if __name__ == "__main__":
    unittest.main()
