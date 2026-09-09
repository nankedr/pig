#!/usr/bin/env python3
"""Run the M4 freeze and package its exact commit for native darwin/arm64."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import tempfile
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[1]


def output(*args):
    return subprocess.check_output(args, cwd=ROOT, text=True).strip()


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: python3 scripts/m4-release.py /absolute/output-directory")
    dest = Path(sys.argv[1]).resolve()
    if dest == ROOT or ROOT in dest.parents:
        raise SystemExit("release output must be outside the checkout")
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise SystemExit("M4 release platform is darwin/arm64")
    if output("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise SystemExit("M4 release requires a clean checkout")
    dest.mkdir(parents=True, exist_ok=True)
    if any(dest.iterdir()):
        raise SystemExit("release output directory must be empty")
    commit = output("git", "rev-parse", "HEAD")
    log = dest / "m4-freeze.log"
    print(f"Running m4-freeze at {commit}; log: {log}", flush=True)
    with log.open("w") as stream:
        result = subprocess.run(["make", "m4-freeze"], cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
    record = {
        "version": "0.4.0", "commit": commit,
        "verified_at": datetime.now(timezone.utc).isoformat(),
        "platform": "darwin/arm64", "go": output("go", "version"),
        "node": output("node", "--version"), "unicode": output("node", "-p", "process.versions.unicode"),
        "code_baseline": "936aff00918de1187f085f123c2812d8f2d67745",
        "catalog_baseline": "53fa77ccd8a279eb87e92294ef3687b03ff80112",
        "command": "make m4-freeze", "exit_code": result.returncode,
        "clean_checkout_after": not output("git", "status", "--porcelain=v1", "--untracked-files=all"),
        "hashes": {"m4-freeze.log": digest(log)},
    }
    verification = dest / "verification.json"
    verification.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    if result.returncode or not record["clean_checkout_after"] or output("git", "rev-parse", "HEAD") != commit:
        raise SystemExit("freeze failed or checkout changed; see m4-freeze.log and verification.json")
    bundle = dest / "pig-v0.4.0-darwin-arm64"
    bundle.mkdir()
    env = {**os.environ, "CGO_ENABLED": "0", "GOOS": "darwin", "GOARCH": "arm64", "GOBIN": str(bundle)}
    subprocess.run(["go", "install", "-trimpath", "./cmd/pig", "./cmd/pig-ai"], cwd=ROOT, env=env, check=True)
    if output(str(bundle / "pig"), "--version") != "0.4.0":
        raise SystemExit("installed CLI version mismatch")
    (dest / "pig-ai-help.txt").write_text(output(str(bundle / "pig-ai"), "--help") + "\n")
    with tempfile.TemporaryDirectory(prefix="pig-m4-sdk-") as temporary:
        sdk = Path(temporary)
        (sdk / "go.mod").write_text(f"module release-check\n\ngo 1.24.0\n\nrequire github.com/nankedr/pig v0.4.0\nreplace github.com/nankedr/pig => {ROOT}\n")
        (sdk / "main.go").write_text('''package main
import ("fmt"; "os"; "github.com/nankedr/pig/codingagent")
func main() {
 dir, err := os.MkdirTemp("", "pig-release-session-"); if err != nil { panic(err) }; defer os.RemoveAll(dir)
 manager, err := codingagent.NewSessionManager(dir, &dir); if err != nil { panic(err) }
 if manager.GetHeader().Version == nil || *manager.GetHeader().Version != 3 { panic("not a v3 Session") }
 fmt.Println(codingagent.Version)
}
''')
        subprocess.run(["go", "mod", "tidy"], cwd=sdk, env=env, check=True)
        version = subprocess.check_output(["go", "run", "."], cwd=sdk, env=env, text=True).strip()
        if version != "0.4.0":
            raise SystemExit("independent SDK install failed")
    with (dest / "installed-html-browser.log").open("w") as stream:
        subprocess.run(["node", "parity/export-html/check.mjs"], cwd=ROOT, env={**env, "PIG_BINARY": str(bundle / "pig")}, stdout=stream, stderr=subprocess.STDOUT, check=True)
    archive = dest / (bundle.name + ".tar.gz")
    with tarfile.open(archive, "w:gz") as tar:
        tar.add(bundle, arcname=bundle.name)
    for path in [ROOT / "parity/catalog.jsonl", ROOT / "parity/catalog.manifest.json", ROOT / "internal/m4gate/testdata/catalog_scope.txt", *sorted((ROOT / "codingagent/testdata").glob("*surface*"))]:
        record["hashes"][str(path.relative_to(ROOT))] = digest(path)
    for path in [*sorted(bundle.iterdir()), archive, dest / "installed-html-browser.log"]:
        record["hashes"][str(path.relative_to(dest))] = digest(path)
    record["install"] = "PASS: local CLI, independent SDK module, installed-binary browser gate"
    record["build_info"] = {path.name: output("go", "version", "-m", str(path)) for path in sorted(bundle.iterdir())}
    if output("git", "rev-parse", "HEAD") != commit or output("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise SystemExit("checkout changed during packaging")
    verification.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    files = [*sorted(bundle.iterdir()), archive, log, verification, dest / "pig-ai-help.txt", dest / "installed-html-browser.log"]
    (dest / "SHA256SUMS").write_text("".join(f"{digest(path)}  {path.relative_to(dest)}\n" for path in files))
    print(f"PASS: {archive}; verification: {verification}")


if __name__ == "__main__":
    main()
