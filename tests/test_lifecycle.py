import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]

class Lifecycle(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="synology-life-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.target = self.root / "target"
        (self.target / "bin").mkdir(parents=True)
        self.data = self.root / "data"
        self.env = dict(os.environ, SYNOPKG_PKGDEST=str(self.target), SYNOPKG_PKGVAR=str(self.data),
                        wizard_ddnsto_token="fixture-token", wizard_ddnsto_idx="7")

    def call(self, name, *args):
        return subprocess.run(["sh", str(ROOT / "scripts" / name), *args], env=self.env,
                              capture_output=True, text=True, timeout=20)

    def test_install_upgrade_keeps_config_and_boot_state(self):
        result = self.call("postinst")
        self.assertEqual(result.returncode, 0, result.stderr)
        config = self.data / "ddnsto-config.json"
        self.assertTrue(config.exists())
        self.assertEqual(json.loads(config.read_text())["routeridx"], 7)
        self.assertEqual(config.stat().st_mode & 0o777, 0o600)
        self.assertTrue((self.target / "bin/ddnsto-config.json").is_symlink())
        (self.data / ".ddnsto_boot.cfg").write_text("fixture-state")
        shutil.rmtree(self.target)
        (self.target / "bin").mkdir(parents=True)
        self.env["wizard_ddnsto_token"] = "replacement-token"
        self.assertEqual(self.call("postinst").returncode, 0)
        self.assertEqual(json.loads(config.read_text())["token"], "fixture-token")
        self.assertEqual((self.target / "bin/.ddnsto_boot.cfg").read_text(), "fixture-state")

    def test_rejects_invalid_index(self):
        self.env["wizard_ddnsto_idx"] = "7, bad-json"
        self.assertNotEqual(self.call("postinst").returncode, 0)

    @unittest.skipUnless(shutil.which("cc"), "C compiler required for process fixture")
    def test_start_status_stop_and_failed_start(self):
        source = self.root / "fixture.c"
        source.write_text('#include <unistd.h>\n#include <stdio.h>\nint main(int n, char **a) { for(int i=1;i<n;i++) puts(a[i]); fflush(stdout); for (;;) pause(); }\n')
        binary = self.target / "bin/ddnsto"
        subprocess.run(["cc", str(source), "-o", str(binary)], check=True)
        self.assertEqual(self.call("start-stop-status", "status").returncode, 3)
        try:
            self.assertEqual(self.call("start-stop-status", "start").returncode, 0)
            self.assertIn("--log-path\n" + str(self.data / "logs"), (self.data / "ddnsto.log").read_text())
            self.assertEqual(self.call("start-stop-status", "status").returncode, 0)
            self.assertEqual(self.call("start-stop-status", "start").returncode, 0)
        finally:
            self.call("start-stop-status", "stop")
        self.assertEqual(self.call("start-stop-status", "status").returncode, 3)
        shutil.copyfile("/bin/false", binary)
        self.assertNotEqual(self.call("start-stop-status", "start").returncode, 0)

if __name__ == "__main__":
    unittest.main()
