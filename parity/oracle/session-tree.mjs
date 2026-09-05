import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-tree.json");

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
	const dir = mkdtempSync(join(tmpdir(), "pi-session-tree-"));
	try {
        const cwd=join(dir,"project"),m=SessionManager.create(cwd,dir,{id:"source"});
        const user=text=>({role:"user",content:text,timestamp:1});
        m.appendSessionInfo("path name");const u=m.appendMessage(user("first"));m.appendLabelChange(u,"checkpoint");
        const a=m.appendMessage({role:"assistant",content:[{type:"text",text:"reply"}],api:"fixture",provider:"fixture",model:"model",usage:{input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:"stop",timestamp:2});
        m.appendMessage(user("abandoned"));m.branch(a);const selected=m.appendMessage(user("selected"));
        m.appendLabelChange(u,"latest");
        const source=m.getSessionFile(),before=readFileSync(source,"utf8");
        const re=SessionManager.open(source);const children=re.getChildren(a).map(e=>e.message.content);
        const labelTime=re.getTree()[0].children[0].labelTimestamp;
        const fork=re.createBranchedSession(selected),opened=SessionManager.open(fork);
        const entries=opened.getEntries(); const ids=new Set(entries.map(e=>e.id));
        const observation={outcome:{children,name:opened.getSessionName(),label:opened.getLabel(u),
            roles:opened.buildSessionContext().messages.map(m=>m.role),texts:opened.buildSessionContext().messages.filter(m=>m.role==="user").map(m=>m.content),
            parentsValid:entries.every(e=>e.parentId===null||ids.has(e.parentId)),labelTimePreserved:opened.getTree()[0].children[0].labelTimestamp===labelTime,
            sourceUnchanged:readFileSync(source,"utf8")===before,parentMatches:opened.getHeader().parentSession===source,idChanged:opened.getSessionId()!==m.getSessionId(),
            leafType:opened.getLeafEntry().type},side_effects:[]};
        opened.appendMessage(user("independent"));observation.outcome.sourceUnchangedAfterAppend=readFileSync(source,"utf8")===before;
        opened.resetLeaf();observation.outcome.resetEmpty=opened.buildSessionContext().messages.length===0;
        const errors=[];for(const action of [()=>opened.branch("missing"),()=>opened.appendLabelChange("missing","x"),()=>opened.createBranchedSession("missing")]){try{action()}catch(e){errors.push(e.message)}}
        observation.outcome.errors=errors;
        const input={projection:"Persist/reopen branching, children, selected-path fork, parent source, name and resolved labels, label timestamp, rewiring and independent writes; invalid entries and reset leaf."};
		const caseValue = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-tree", catalog_id: "contract:session/v3-jsonl", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
		const fixture = {
			schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
			upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference },
			case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: observationDigest(observation),
			execution_method: "node --experimental-strip-types parity/oracle/session-tree.mjs <locked-pi-checkout>",
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
