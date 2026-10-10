#!/usr/bin/env python3
"""Build SPKs from prebuilt Linux ELF binaries; Python is only needed on the build host."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def git_state(path):
    def git(*args):
        result = subprocess.run(["git", "-C", str(path), *args], capture_output=True, text=True)
        return result.stdout.strip() if result.returncode == 0 else None
    return {"commit": git("rev-parse", "HEAD"), "status": git("status", "--porcelain")}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", choices=["ddnsto_arm", "ddnsto_x86"])
    parser.add_argument("--dsm", choices=["dsm6", "dsm7"])
    args = parser.parse_args()
    if bool(args.package) != bool(args.dsm):
        parser.error("--package and --dsm must be supplied together")
    inputs = Path(os.environ.get("DDNSTO_PATH", ROOT.parent / "ddns-client/bin/cli_ddnsto/synology")).resolve()
    version = (inputs / "latest").read_text().strip()
    revision = os.environ.get("PKG_REVISION", "1")
    if not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version) or not re.fullmatch(r"[1-9][0-9]*", revision):
        raise ValueError("Invalid version or PKG_REVISION")
    package_version = version + "-" + revision
    output = Path(os.environ.get("OUT_DIR", ROOT / "dist" / package_version)).resolve()
    targets = [(args.package, args.dsm)] if args.package else [
        ("ddnsto_" + arch, dsm) for arch in ("arm", "x86") for dsm in ("dsm6", "dsm7")]
    binaries = {}
    # Validate every input before writing any package.
    for name, _ in targets:
        arch = name.split("_")[-1]
        if arch in binaries:
            continue
        filename, elf_class, machine = ("synology.arm", 1, 40) if arch == "arm" else ("synology.amd64", 2, 62)
        binary = Path(os.environ.get("ARM_BIN" if arch == "arm" else "X86_BIN", inputs / filename)).resolve()
        with binary.open("rb") as stream:
            header = stream.read(64)
        if (len(header) < 52 or header[:4] != b"\x7fELF" or header[4:6] != bytes([elf_class, 1])
                or int.from_bytes(header[18:20], "little") != machine):
            raise ValueError("Wrong ELF architecture: " + str(binary))
        metadata = Path(str(binary) + ".json")
        if metadata.exists():
            data = json.loads(metadata.read_text())
            if data.get("version") != version or data.get("hash") != sha256(binary):
                raise ValueError("Binary version/hash mismatch: " + str(binary))
        binaries[arch] = binary
    output.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".build-", dir=output) as tmp:
        staging = Path(tmp)
        artifacts = []
        for name, dsm in targets:
            arch = name.split("_")[-1]
            work = staging / (name + "-" + dsm)
            work.mkdir()
            payload = work / "payload"
            shutil.copytree(ROOT / "package", payload)
            (payload / "bin").mkdir(exist_ok=True)
            shutil.copyfile(binaries[arch], payload / "bin/ddnsto")
            (payload / "bin/ddnsto").chmod(0o755)
            info = (ROOT / arch / ("INFO-" + dsm)).read_text()
            info, count = re.subn(r'^version="[^"]*"$', 'version="' + package_version + '"', info, flags=re.M)
            if count != 1:
                raise ValueError("INFO must have exactly one version field")
            (work / "INFO").write_text(info)
            for item in ("scripts", "WIZARD_UIFILES", "CHANGELOG", "LICENSE", "PACKAGE_ICON.PNG", "PACKAGE_ICON_256.PNG", "conf"):
                source = ROOT / item
                if source.is_dir():
                    shutil.copytree(source, work / item)
                else:
                    shutil.copyfile(source, work / item)
            with tarfile.open(work / "package.tgz", "w:gz", format=tarfile.USTAR_FORMAT) as archive:
                for item in sorted(payload.iterdir()):
                    archive.add(item, arcname=item.name)
            spk = staging / (name + "-" + dsm + ".spk")
            with tarfile.open(spk, "w", format=tarfile.USTAR_FORMAT) as archive:
                for item in sorted(work.iterdir()):
                    if item != payload:
                        archive.add(item, arcname=item.name)
            artifacts.append(spk)
        manifest_name = "build-manifest.json" if not args.package else args.package + "-" + args.dsm + "-manifest.json"
        manifest = staging / manifest_name
        manifest.write_text(json.dumps({
            "client_version": version, "package_version": package_version,
            "package_source": git_state(ROOT), "client_source": git_state(ROOT.parent / "ddns-client"),
            "binaries": {arch: {"path": str(path), "sha256": sha256(path)} for arch, path in binaries.items()},
            "artifacts": {path.name: sha256(path) for path in artifacts},
        }, indent=2) + "\n")
        sums = staging / ("SHA256SUMS" if not args.package else args.package + "-" + args.dsm + ".sha256")
        sums.write_text("".join(sha256(path) + "  " + path.name + "\n" for path in artifacts + [manifest]))
        for path in artifacts + [manifest, sums]:
            path.replace(output / path.name)
    print("Built " + str(len(targets)) + " SPK(s): " + str(output))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, tarfile.TarError) as error:
        raise SystemExit("Packaging failed: " + str(error))
