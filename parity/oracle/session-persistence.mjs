import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-persistence.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
	}
	return value;
}

function caseDigest(value) {
	const projected = {
		schema_version: value.schema_version,
		id: value.id,
		catalog_id: value.catalog_id,
		surface: value.surface,
		input: canonical(value.input),
		observe: value.observe,
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function parseArgs(argv) {
	const result = { pi: defaultPi, out: defaultOutput, check: false };
	for (let i = 2; i < argv.length; i++) {
		if (argv[i] === "--check") result.check = true;
		else if (argv[i] === "--out") result.out = argv[++i];
		else if (result.pi === defaultPi) result.pi = argv[i];
		else throw new Error(`unexpected argument: ${argv[i]}`);
	}
	return result;
}

function projectEntries(entries) {
	const ids = new Map(entries.map((entry, index) => [entry.id, index]));
	return entries.map((entry) => ({
		type: entry.type,
		role: entry.type === "message" ? entry.message.role : undefined,
		provider: entry.provider,
		modelId: entry.modelId,
		thinkingLevel: entry.thinkingLevel,
		parent: entry.parentId === null ? null : ids.get(entry.parentId),
	}));
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
	const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
	const reference = "packages/coding-agent/src/core/session-manager.ts";
	const { SessionManager } = await import(pathToFileURL(join(args.pi, reference)).href);
	const dir = mkdtempSync(join(tmpdir(), "pi-session-persistence-"));
	try {
		const cwd = join(dir, "project");
		const manager = SessionManager.create(cwd, dir, { id: "m3-session" });
		const file = manager.getSessionFile();
		const existence = [existsSync(file)];
		manager.appendModelChange("fixture", "model-1");
		existence.push(existsSync(file));
		manager.appendThinkingLevelChange("high");
		existence.push(existsSync(file));
		manager.appendMessage({ role: "user", content: [{ type: "text", text: "first" }], timestamp: 1 });
		existence.push(existsSync(file));
		manager.appendMessage({
			role: "assistant",
			content: [{ type: "text", text: "partial" }],
			api: "fixture",
			provider: "fixture",
			model: "model-1",
			usage: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
			stopReason: "error",
			errorMessage: "provider failed",
			timestamp: 2,
		});
		existence.push(existsSync(file));
		manager.appendMessage({ role: "toolResult", toolCallId: "call-1", toolName: "read", content: [{ type: "text", text: "tool output" }], isError: false, timestamp: 3 });

		const firstRecords = readFileSync(file, "utf8").trim().split("\n").map(JSON.parse);
		const reopened = SessionManager.open(file);
		const beforeAppend = readFileSync(file, "utf8");
		reopened.appendMessage({ role: "user", content: [{ type: "text", text: "second" }], timestamp: 4 });
		const afterAppend = readFileSync(file, "utf8");

		const memory = SessionManager.inMemory(cwd, { id: "memory-session" });
		memory.appendMessage({ role: "user", content: "memory", timestamp: 5 });

		const emptyPath = join(dir, "empty.jsonl");
		writeFileSync(emptyPath, "");
		const empty = SessionManager.open(emptyPath);
		const invalidPath = join(dir, "invalid.jsonl");
		const invalidContent = '{"type":"message","id":"orphan"}\n';
		writeFileSync(invalidPath, invalidContent);
		let invalidError = "";
		try { SessionManager.open(invalidPath); } catch (error) { invalidError = error.message; }

		const observation = {
			outcome: {
				created: {
					id: manager.getSessionId(),
					fileHasID: basename(file).endsWith("_m3-session.jsonl"),
					existence,
					records: projectEntries(firstRecords.slice(1)),
					header: { type: firstRecords[0].type, version: firstRecords[0].version, id: firstRecords[0].id, cwdMatches: firstRecords[0].cwd === cwd },
				},
				reopened: {
					id: reopened.getSessionId(),
					roles: reopened.buildSessionContext().messages.map((message) => message.role),
					unchangedBeforeAppend: beforeAppend === readFileSync(file, "utf8").slice(0, beforeAppend.length),
					appendedLines: afterAppend.trim().split("\n").length - beforeAppend.trim().split("\n").length,
				},
				memory: { persisted: memory.isPersisted(), file: memory.getSessionFile() ?? null },
				empty: { idMatchesHeader: empty.getSessionId() === empty.getHeader().id, lines: readFileSync(emptyPath, "utf8").trim().split("\n").length },
				invalid: { failed: invalidError.includes("not a valid pi session"), preserved: readFileSync(invalidPath, "utf8") === invalidContent },
			},
			side_effects: [
				{ kind: "file-create", target: "session", detail: "after-first-assistant" },
				{ kind: "file-write", target: "explicit-empty", detail: "header" },
				{ kind: "file-preserve", target: "explicit-invalid", detail: "unchanged" },
			],
		};
		const input = { id: "m3-session", projection: "File creation timing, v3 header, model/thinking/messages/tool result parent chain, explicit reopen append, memory mode, empty and invalid explicit paths; random IDs and timestamps are projected." };
		const caseValue = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-persistence", catalog_id: "contract:session/v3-jsonl", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
		const fixture = {
			schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
			upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference },
			case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: observationDigest(observation),
			execution_method: "node --experimental-strip-types parity/oracle/session-persistence.mjs <locked-pi-checkout>",
			platform: "any", environment: { node: process.version, oracle_entry: reference },
		};
		if (args.check) {
			const committed = JSON.parse(readFileSync(args.out, "utf8"));
			fixture.environment.node = committed.environment.node;
			if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error(`committed fixture does not reproduce: ${args.out}`);
			console.log(`verified ${args.out}`);
		} else {
			writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
			console.log(`wrote ${args.out}`);
		}
	} finally {
		rmSync(dir, { recursive: true, force: true });
	}
}

main().catch((error) => { console.error(error); process.exit(1); });
