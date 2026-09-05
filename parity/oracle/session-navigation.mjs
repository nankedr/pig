import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-navigation.json");

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
	const dir = mkdtempSync(join(tmpdir(), "pi-session-navigation-"));
	try {
        const cwd = join(dir, "project"), other = join(dir, "other");
        const assistant = (timestamp) => ({ role: "assistant", content: [{type: "text", text: "reply"}], api: "fixture", provider: "fixture", model: "model", usage: {input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}}, stopReason:"stop", timestamp });
        const make = (id, project, timestamp) => {
            const m = SessionManager.create(project, dir, {id});
            m.appendMessage({role:"user", content:"hello", timestamp:timestamp-1});
            m.appendMessage(assistant(timestamp));
            return m;
        };
        const a = make("local-a", cwd, 2001), b = make("local-b", cwd, 3001), c = make("other", other, 4001);
        a.appendSessionInfo("  named\r\n session  ");
        b.appendSessionInfo("cleared"); b.appendSessionInfo(" ");
        writeFileSync(join(dir,"invalid.jsonl"), "invalid");
        const {utimesSync} = await import("node:fs");
        utimesSync(a.getSessionFile(), 10, 10); utimesSync(b.getSessionFile(), 5, 5); utimesSync(c.getSessionFile(), 20, 20);
        const progress = [];
        const local = await SessionManager.list(cwd, dir, (n,total) => progress.push([n,total]));
        const all = await SessionManager.listAll(dir);
        const observation = {outcome: {
            local: local.map(s=>({id:s.id,name:s.name??null,modified:s.modified.getTime(),count:s.messageCount,first:s.firstMessage,text:s.allMessagesText})),
            all: all.map(s=>s.id), recent:SessionManager.continueRecent(cwd,dir).getSessionId(), progress,
        }, side_effects: []};
        const input = {projection:"Current-CWD and flat all-session discovery; names, message activity sorting versus mtime continue; corrupt-file progress."};
		const caseValue = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-navigation", catalog_id: "contract:session/v3-jsonl", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
		const fixture = {
			schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
			upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference },
			case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: observationDigest(observation),
			execution_method: "node --experimental-strip-types parity/oracle/session-navigation.mjs <locked-pi-checkout>",
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
