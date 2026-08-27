// Captures the first real CLI Parity Case through Pi's public process entry.

import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "codingagent-auth-help.json");

function parseArgs(argv) {
	const result = { piRoot: defaultPiRoot, out: defaultOutput, check: false, install: false };
	const args = argv.slice(2);
	for (let index = 0; index < args.length; index++) {
		if (args[index] === "--check") result.check = true;
		else if (args[index] === "--install") result.install = true;
		else if (args[index] === "--out") result.out = args[++index];
		else if (result.piRoot === defaultPiRoot) result.piRoot = args[index];
		else throw new Error(`unexpected argument: ${args[index]}`);
	}
	return result;
}

function loadLock() {
	return JSON.parse(readFileSync(join(repoRoot, "parity", "baseline", "upstream.lock.json"), "utf8"));
}

function assertLockedCheckout(piRoot, commit) {
	let head;
	try {
		head = execFileSync("git", ["-C", piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	} catch (error) {
		throw new Error(`Pi checkout is unavailable at ${piRoot}; provision it with parity/oracle/run.mjs --fetch: ${error.message}`);
	}
	if (head !== commit) throw new Error(`Pi checkout HEAD ${head} != locked commit ${commit}`);
	const dirty = spawnSync("git", ["-C", piRoot, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
	if (dirty.error || dirty.status !== 0) throw dirty.error ?? new Error(`unable to inspect Pi tracked files`);
	if (dirty.stdout !== "") throw new Error(`Pi checkout has tracked changes; Oracle input must exactly match ${commit}`);
}

function digest(value) {
	return `sha256:${createHash("sha256").update(JSON.stringify(value)).digest("hex")}`;
}

function fileDigest(path) {
	return `sha256:${createHash("sha256").update(readFileSync(path)).digest("hex")}`;
}

function runCLI(command, args, work, env) {
	const run = spawnSync(command, args, { cwd: work, encoding: "utf8", env });
	if (run.error) throw run.error;
	return {
		stdout: run.stdout,
		stderr: run.stderr,
		exit_status: run.status === null ? { signal: run.signal ?? "UNKNOWN" } : { code: run.status },
	};
}

function capture(piRoot, lock) {
	const work = mkdtempSync(join(tmpdir(), "pig-parity-auth-help-"));
	try {
		const home = join(work, "home");
		const temp = join(work, "tmp");
		mkdirSync(home);
		mkdirSync(temp);
		const cli = join(piRoot, "packages", "coding-agent", "dist", "cli.js");
		if (!existsSync(cli)) {
			throw new Error(`Pi public bin is absent at ${cli}; build the locked checkout with its offline build before capture`);
		}
		const sourceCLI = join(piRoot, "packages", "coding-agent", "src", "cli.ts");
		const authCommand = join(piRoot, "packages", "coding-agent", "src", "cli", "auth-command.ts");
		const env = {
			...process.env,
			HOME: home,
			TMPDIR: temp,
			NO_COLOR: "1",
			FORCE_COLOR: "0",
			PI_OFFLINE: "1",
			PI_SKIP_VERSION_CHECK: "1",
		};
		const observation = runCLI(cli, ["auth", "--help"], work, env);
		const sourceObservation = runCLI(process.execPath, [sourceCLI, "auth", "--help"], work, env);
		if (JSON.stringify(observation) !== JSON.stringify(sourceObservation)) {
			throw new Error(`Pi public bin does not match the clean locked source entry for auth --help`);
		}
		if (observation.exit_status.code !== 0) {
			throw new Error(`Pi auth help exited ${JSON.stringify(observation.exit_status)}: ${observation.stderr}`);
		}
		const caseDeclaration = {
			schema_version: "1.0.0",
			id: "cli/pig/auth-help",
			catalog_id: "contract:cli/pig/auth-help",
			surface: "cli",
			input: { arguments: ["auth", "--help"] },
			observe: ["stdout", "stderr", "exit_status"],
			normalizations: [
				{ target: "/stdout", kind: "brand-token", oracle: "pi", pig: "pig", exact_matches: 3 },
			],
		};
		return {
			schema_version: "1.0.0",
			deterministic: true,
			baseline_id: lock.baseline_id,
			baseline_commit: lock.upstream.commit,
			upstream: {
				repository: lock.upstream.repository,
				commit: lock.upstream.commit,
				reference: "packages/coding-agent/src/cli/auth-command.ts#printAuthCommandHelp",
			},
			case: caseDeclaration,
			observation,
			input_hash: digest(caseDeclaration),
			observation_hash: digest(observation),
			execution_method: "packages/coding-agent/dist/cli.js auth --help; byte-checked against clean locked src/cli.ts",
			platform: `${process.platform}-${process.arch}`,
			environment: {
				node: process.version,
				platform: process.platform,
				arch: process.arch,
				public_bin_sha256: fileDigest(cli),
				source_entry_sha256: fileDigest(sourceCLI),
				auth_command_sha256: fileDigest(authCommand),
			},
		};
	} finally {
		rmSync(work, { recursive: true, force: true });
	}
}

function main() {
	const args = parseArgs(process.argv);
	const lock = loadLock();
	assertLockedCheckout(args.piRoot, lock.upstream.commit);
	if (args.install) execFileSync("npm", ["ci", "--ignore-scripts"], { cwd: args.piRoot, stdio: "inherit" });
	if (!existsSync(join(args.piRoot, "node_modules", "chalk", "package.json"))) {
		throw new Error(`Pi dependencies are absent; rerun with --install (only the on-demand Oracle needs them)`);
	}
	const fixture = capture(args.piRoot, lock);
	if (args.check) {
		const recorded = JSON.parse(readFileSync(args.out, "utf8"));
		fixture.environment.node = recorded.environment.node;
		fixture.environment.platform = recorded.environment.platform;
		fixture.environment.arch = recorded.environment.arch;
		fixture.platform = recorded.platform;
		if (JSON.stringify(fixture) !== JSON.stringify(recorded)) throw new Error(`fixture does not reproduce: ${args.out}`);
		console.log(`verified ${args.out}`);
		return;
	}
	mkdirSync(dirname(args.out), { recursive: true });
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.log(`wrote ${args.out}`);
}

main();
