#!/usr/bin/env python3
"""Run the M6 freeze and package its exact commit for native darwin/arm64."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
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
        raise SystemExit("usage: python3 scripts/m6-release.py /absolute/output-directory")
    dest = Path(sys.argv[1]).resolve()
    if dest == ROOT or ROOT in dest.parents:
        raise SystemExit("release output must be outside the checkout")
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise SystemExit("M6 release platform is darwin/arm64")
    if output("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise SystemExit("M6 release requires a clean checkout")
    dest.mkdir(parents=True, exist_ok=True)
    if any(dest.iterdir()):
        raise SystemExit("release output directory must be empty")
    commit = output("git", "rev-parse", "HEAD")
    manual = Path(os.environ.get("PIG_M6_MANUAL_EVIDENCE", "")).resolve()
    subprocess.run([sys.executable, str(ROOT / "scripts/m6-evidence.py"), str(manual)], check=True)
    manual_record = json.loads(manual.read_text())
    log = dest / "m6-freeze.log"
    print(f"Running m6-freeze at {commit}; log: {log}", flush=True)
    with log.open("w") as stream:
        result = subprocess.run(["make", "m6-freeze"], cwd=ROOT, stdout=stream, stderr=subprocess.STDOUT)
    record = {
        "version": "0.6.0", "commit": commit,
        "verified_at": datetime.now(timezone.utc).isoformat(),
        "platform": "darwin/arm64", "go": output("go", "version"), "go_flags": os.environ.get("GOFLAGS", ""),
        "node": output("node", "--version"), "unicode": output("node", "-p", "process.versions.unicode"),
        "code_baseline": "936aff00918de1187f085f123c2812d8f2d67745",
        "catalog_baseline": "53fa77ccd8a279eb87e92294ef3687b03ff80112",
        "command": "make m6-freeze", "exit_code": result.returncode,
        "manual_acceptance": {"status": manual_record.get("status", "passed"), "record": "manual-evidence/evidence.json", "reason": manual_record.get("reason", "human terminal evidence validated")},
        "clean_checkout_after": not output("git", "status", "--porcelain=v1", "--untracked-files=all"),
        "hashes": {"m6-freeze.log": digest(log)},
        "scope": "M6 delivered text slices only; inventoried/scaffolded APIs and unsupported partial branches remain unfrozen; #99 unscheduled, M7/M11/M12/M13 deferred",
    }
    verification = dest / "verification.json"
    verification.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    if result.returncode or not record["clean_checkout_after"] or output("git", "rev-parse", "HEAD") != commit:
        raise SystemExit("freeze failed or checkout changed; see m6-freeze.log and verification.json")
    bundle = dest / "pig-v0.6.0-darwin-arm64"
    bundle.mkdir()
    env = {**os.environ, "CGO_ENABLED": "0", "GOOS": "darwin", "GOARCH": "arm64", "GOBIN": str(bundle)}
    subprocess.run(["go", "install", "-trimpath", "./cmd/pig", "./cmd/pig-ai"], cwd=ROOT, env=env, check=True)
    if output(str(bundle / "pig"), "--version") != "0.6.0":
        raise SystemExit("installed CLI version mismatch")
    (dest / "pig-ai-help.txt").write_text(output(str(bundle / "pig-ai"), "--help") + "\n")
    artifacts = dest / "terminal-artifacts"
    env["PIG_M6_ARTIFACTS"] = str(artifacts)
    with tempfile.TemporaryDirectory(prefix="pig-m6-sdk-") as temporary:
        sdk = Path(temporary)
        (sdk / "go.mod").write_text(f"module release-check\n\ngo 1.24.0\n\nrequire github.com/nankedr/pig v0.6.0\nreplace github.com/nankedr/pig => {ROOT}\n")
        shutil.copy2(ROOT / "examples/m6-workflow/main.go", sdk / "main.go")
        subprocess.run(["go", "mod", "tidy"], cwd=sdk, env=env, check=True)
        sdk_binary = sdk / "m6-sdk"
        subprocess.run(["go", "build", "-o", str(sdk_binary), "."], cwd=sdk, env=env, check=True)
        with (dest / "sdk-workflow.log").open("w") as stream:
            subprocess.run(["go", "test", "./internal/m6gate", "-run", "^TestM6SDKWorkflow$", "-count=1", "-v"], cwd=ROOT, env={**env, "PIG_M6_SDK_BINARY": str(sdk_binary)}, stdout=stream, stderr=subprocess.STDOUT, check=True)
    with (dest / "installed-cli-workflow.log").open("w") as stream:
        subprocess.run(["go", "test", "./internal/m6gate", "-run", "^TestPigM6Workflow$", "-count=1", "-v"], cwd=ROOT, env={**env, "PIG_BINARY": str(bundle / "pig")}, stdout=stream, stderr=subprocess.STDOUT, check=True)
    with (dest / "installed-html-browser.log").open("w") as stream:
        subprocess.run(["node", "parity/export-html/check.mjs"], cwd=ROOT, env={**env, "PIG_BINARY": str(bundle / "pig")}, stdout=stream, stderr=subprocess.STDOUT, check=True)
    for name in ["LICENSE", "THIRD_PARTY_NOTICES", "README.md"]:
        shutil.copy2(ROOT / name, bundle / name)
    shutil.copytree(ROOT / "THIRD_PARTY_LICENSES", bundle / "THIRD_PARTY_LICENSES")
    html_license = bundle / "codingagent/exporthtml/LICENSES.txt"
    html_license.parent.mkdir(parents=True)
    shutil.copy2(ROOT / "codingagent/exporthtml/LICENSES.txt", html_license)
    shutil.copy2(ROOT / "docs/releases/v0.6.0.md", bundle / "RELEASE_NOTES.md")
    bundle_files = sorted(path for path in bundle.rglob("*") if path.is_file())
    archive = dest / (bundle.name + ".tar.gz")
    with tarfile.open(archive, "w:gz") as tar:
        tar.add(bundle, arcname=bundle.name)
    source = dest / "pig-v0.6.0-source.tar.gz"
    subprocess.run(["git", "archive", "--format=tar.gz", "--prefix=pig-v0.6.0/", "-o", str(source), commit], cwd=ROOT, check=True)
    manual_dir = dest / "manual-evidence"
    manual_dir.mkdir()
    shutil.copy2(manual, manual_dir / "evidence.json")
    for case in json.loads(manual.read_text())["cases"]:
        for artifact in case["artifacts"]:
            relative = Path(artifact["path"])
            if relative.is_absolute() or ".." in relative.parts:
                raise SystemExit("manual artifact paths must be relative to the evidence directory")
            target = manual_dir / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(manual.parent / relative, target)
    for path in [ROOT / "parity/catalog.jsonl", ROOT / "parity/catalog.manifest.json", ROOT / "internal/m6gate/testdata/catalog_scope.txt", *sorted((ROOT / "codingagent/testdata").glob("*surface*")), *sorted((ROOT / "codingagent/testdata").glob("*.golden.txt"))]:
        record["hashes"][str(path.relative_to(ROOT))] = digest(path)
    for path in [*bundle_files, archive, source, *sorted(artifacts.rglob("*")), *sorted(manual_dir.rglob("*")), dest / "installed-html-browser.log", dest / "installed-cli-workflow.log", dest / "sdk-workflow.log"]:
        if path.is_file():
            record["hashes"][str(path.relative_to(dest))] = digest(path)
    record["install"] = "PASS: installed CLI workflow, independent SDK workflow, installed-binary browser gate; public version installation verified separately after tagging"
    record["build_info"] = {path.name: output("go", "version", "-m", str(path)) for path in [bundle / "pig", bundle / "pig-ai"]}
    if output("git", "rev-parse", "HEAD") != commit or output("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise SystemExit("checkout changed during packaging")
    verification.write_text(json.dumps(record, ensure_ascii=False, indent=2) + "\n")
    files = [*bundle_files, archive, source, *sorted(p for p in artifacts.rglob("*") if p.is_file()), *sorted(p for p in manual_dir.rglob("*") if p.is_file()), log, verification, dest / "pig-ai-help.txt", dest / "installed-html-browser.log", dest / "installed-cli-workflow.log", dest / "sdk-workflow.log"]
    (dest / "SHA256SUMS").write_text("".join(f"{digest(path)}  {path.relative_to(dest)}\n" for path in files))
    print(f"PASS: {archive}; verification: {verification}")


def verify_published(dest):
    record = json.loads((dest / "verification.json").read_text())
    if record["version"] != "0.6.0" or record["exit_code"] or not record.get("install"):
        raise SystemExit("a successful v0.6.0 freeze/package record is required")
    if output("git", "rev-parse", "HEAD") != record["commit"] or output("git", "status", "--porcelain=v1", "--untracked-files=all"):
        raise SystemExit("published install verification requires the clean frozen checkout")
    subprocess.run([sys.executable, str(ROOT / "scripts/m6-evidence.py"), str(dest / "manual-evidence/evidence.json")], check=True)
    with tempfile.TemporaryDirectory(prefix="pig-m6-public-") as temporary:
        work = Path(temporary)
        sdk, binaries = work / "sdk", work / "bin"
        sdk.mkdir()
        env = {**os.environ, "GOWORK": "off", "GOFLAGS": "", "CGO_ENABLED": "0", "GOBIN": str(binaries), "GOMODCACHE": str(work / "modules"), "GOPROXY": "https://proxy.golang.org,direct", "GOSUMDB": "sum.golang.org"}
        log = dest / "published-install.log"
        with log.open("w") as stream:
            def run(args, cwd=sdk):
                subprocess.run(args, cwd=cwd, env=env, stdout=stream, stderr=subprocess.STDOUT, check=True)
            run(["go", "install", "github.com/nankedr/pig/cmd/pig@v0.6.0"])
            run(["go", "install", "github.com/nankedr/pig/cmd/pig-ai@v0.6.0"])
            run(["go", "mod", "init", "published-sdk-check"])
            run(["go", "get", "github.com/nankedr/pig@v0.6.0"])
            shutil.copy2(ROOT / "examples/m6-workflow/main.go", sdk / "main.go")
            run(["go", "mod", "tidy"])
            sdk_binary = sdk / "m6-sdk"
            run(["go", "build", "-o", str(sdk_binary), "."])
            subprocess.run(["go", "test", "./internal/m6gate", "-run", "^TestM6SDKWorkflow$", "-count=1", "-v"], cwd=ROOT, env={**env, "PIG_M6_SDK_BINARY": str(sdk_binary), "PIG_M6_ARTIFACTS": str(dest / "published-terminal-artifacts")}, stdout=stream, stderr=subprocess.STDOUT, check=True)
            run([str(binaries / "pig-ai"), "--help"])
            version = subprocess.check_output([str(binaries / "pig"), "--version"], env=env, text=True).strip()
            if version != "0.6.0" or "replace " in (sdk / "go.mod").read_text():
                raise SystemExit("published version mismatch or local replacement")
            installed = {**env, "PIG_BINARY": str(binaries / "pig"), "PIG_M6_ARTIFACTS": str(dest / "published-terminal-artifacts")}
            subprocess.run(["go", "test", "./internal/m6gate", "-run", "^TestPigM6Workflow$", "-count=1", "-v"], cwd=ROOT, env=installed, stdout=stream, stderr=subprocess.STDOUT, check=True)
            subprocess.run(["node", "parity/export-html/check.mjs"], cwd=ROOT, env=installed, stdout=stream, stderr=subprocess.STDOUT, check=True)
        metadata = json.loads(subprocess.check_output(["go", "mod", "download", "-json", "github.com/nankedr/pig@v0.6.0"], cwd=sdk, env=env, text=True))
        info = json.loads(Path(metadata["Info"]).read_text())
        if info.get("Origin", {}).get("Hash") != record["commit"]:
            raise SystemExit("published module origin does not match frozen commit")
        public = {"version": version, "commit": record["commit"], "verified_at": datetime.now(timezone.utc).isoformat(), "module": metadata["Path"], "sum": metadata["Sum"], "go_mod_sum": metadata["GoModSum"], "origin": info["Origin"], "result": "PASS: fresh public CLI/SDK installation without replace; local workflows and browser export", "log_sha256": digest(log)}
        proof = dest / "published-install.json"
        proof.write_text(json.dumps(public, ensure_ascii=False, indent=2) + "\n")
        sums = dest / "SHA256SUMS"
        lines = [line for line in sums.read_text().splitlines() if not line.endswith(("  published-install.log", "  published-install.json"))]
        captures = sorted(p for p in (dest / "published-terminal-artifacts").rglob("*") if p.is_file())
        lines = [line for line in lines if "  published-terminal-artifacts/" not in line]
        sums.write_text("\n".join(lines) + "\n" + "".join(f"{digest(path)}  {path.relative_to(dest)}\n" for path in [log, proof, *captures]))
        print("PASS: published v0.6.0 CLI/SDK and exact frozen commit")


if __name__ == "__main__":
    if len(sys.argv) == 3 and sys.argv[1] == "--verify-published":
        verify_published(Path(sys.argv[2]).resolve())
    else:
        main()
