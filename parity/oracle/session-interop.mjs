import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, dirname } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const pi = process.argv[2] ?? join(root, ".upstream/pi");
const check = process.argv.includes("--check");
const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json")));
const reference = "packages/coding-agent/src/core/session-manager.ts";
if (execFileSync("git", ["-C", pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim() !== lock.upstream.commit) throw Error("wrong Pi baseline");
const { SessionManager } = await import(pathToFileURL(join(pi, reference)));
const { convertToLlm } = await import(pathToFileURL(join(pi, "packages/coding-agent/src/core/messages.ts")));
const dir = mkdtempSync(join(tmpdir(), "pig-session-interop-"));
const timestamp = "2025-01-01T00:00:00.000Z";
const user = { role: "user", content: "continued", timestamp: 5 };
const assistant = { role: "assistant", content: [{ type: "text", text: "hello" }], api: "fixture", provider: "fixture", model: "model-1", usage: { input: 1, output: 1, cacheRead: 0, cacheWrite: 0, totalTokens: 2, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } }, stopReason: "stop", timestamp: 2 };
function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([k, v]) => [k, canonical(v)]));
	return value;
}
function canonicalNumber(value) {
	if (!Number.isFinite(value)) throw new Error(`non-finite JSON number: ${value}`);
	if (value === 0) return "0";
	let text = String(value).toLowerCase();
	let sign = "";
	if (text.startsWith("-")) { sign = "-"; text = text.slice(1); }
	const [mantissa, exponentText = "0"] = text.split("e");
	const [integer, fraction = ""] = mantissa.split(".");
	let digits = `${integer}${fraction}`.replace(/^0+/, "");
	if (digits === "") return "0";
	let exponent = Number(exponentText) - fraction.length;
	const trimmed = digits.replace(/0+$/, "");
	exponent += digits.length - trimmed.length;
	digits = trimmed;
	return `${sign}${digits}${exponent === 0 ? "" : `e${exponent}`}`;
}

function canonicalJSONString(value) {
	if (value === null) return "null";
	if (typeof value === "number") return canonicalNumber(value);
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value).replace(/\u2028/g, "\\u2028").replace(/\u2029/g, "\\u2029");
	if (Array.isArray(value)) return `[${value.map(canonicalJSONString).join(",")}]`;
	if (typeof value === "object") return `{${Object.entries(value).map(([key, child]) => `${JSON.stringify(key)}:${canonicalJSONString(child)}`).join(",")}}`;
	throw new Error(`unsupported JSON value: ${typeof value}`);
}

const hash = (value) => `sha256:${createHash("sha256").update(canonicalJSONString(value)).digest("hex")}`;
// Only generated entry IDs/references, entry/header timestamps and generated header cwd are normalized.
function normalize(records, { generated = true, generatedHeader = false, generatedTimestamps = true, firstGenerated = 1 } = {}) {
	const ids = new Map(records.slice(firstGenerated).map((entry, i) => [entry.id, `entry-${i + firstGenerated - 1}`]));
	return records.map((entry, i) => {
		const out = structuredClone(entry);
		if (i === 0 && generatedHeader) { out.timestamp = timestamp; out.cwd = "/project"; }
		if (i >= firstGenerated && generated) {
			for (const key of ["id", "parentId", "firstKeptEntryId", "fromId", "targetId"]) if (ids.has(out[key])) out[key] = ids.get(out[key]);
			if (generatedTimestamps) out.timestamp = timestamp;
		}
		return out;
	});
}
const jsonl = (records) => records.map(JSON.stringify).join("\n") + "\n";
const read = (path) => readFileSync(path, "utf8").split("\n").flatMap((line) => { try { return [JSON.parse(line)]; } catch { return []; } });
try {
	const writer = SessionManager.create("/project", dir, { id: "pi-writer" });
	writer.appendModelChange("fixture", "model-1");
	writer.appendThinkingLevelChange("high");
	writer.appendCompaction("superseded summary", "missing", 7);
	const kept = writer.appendMessage({ role: "user", content: "hi", timestamp: 1 });
	writer.appendMessage(assistant);
	writer.appendMessage({ role: "toolResult", toolCallId: "call-1", toolName: "read", content: [{ type: "text", text: "result" }], isError: false, timestamp: 3 });
	const mainLeaf = writer.getLeafId();
	writer.branch(kept);
	writer.appendMessage({ role: "user", content: "discarded sibling", timestamp: 3 });
	writer.branch(mainLeaf);
	writer.appendCompaction("old summary", kept, 10, { source: "fixture" });
	writer.appendCustomEntry("state", { nested: [1, { keep: true }] });
	writer.appendCustomMessageEntry("note", "custom text", true, { keep: "details" });
	writer.appendLabelChange(kept, "old label");
	writer.appendLabelChange(kept, undefined);
	writer.appendLabelChange(kept, "keep");
	writer.appendSessionInfo("history");
	writer.branchWithSummary(writer.getLeafId(), "branch summary", { details: { keep: true } });
	writer.appendMessage({ role: "futureRole", content: [{ type: "futureBlock", payload: { keep: 1 } }], timestamp: 4, unknown: [1, 2] });
	const all = normalize(read(writer.getSessionFile()), { generatedHeader: true });
	// Unknown fields are provided as historical input, independent of Pig's decoder.
	const labelTarget = all.find((e) => e.type === "label").targetId;
	const legacy = structuredClone(all);
	legacy[0] = { ...legacy[0], id: "legacy", version: 2, futureHeader: { keep: 7 } };
	legacy[2].futureEntry = { keep: [1, 2] };
	legacy.at(-1).message.futureMessage = "preserved";
	legacy.push({ type: "message", id: "hook", parentId: legacy.at(-1).id, timestamp, message: { role: "hookMessage", customType: "legacy", content: "hook text", display: true, timestamp: 4, future: { keep: true } } });
	legacy.slice(1).forEach((entry, i) => { entry.timestamp = new Date(Date.parse(timestamp) + (i + 1) * 1000).toISOString(); });
	const v1 = structuredClone(legacy);
	delete v1[0].version;
	for (const entry of v1.slice(1)) { delete entry.id; delete entry.parentId; if (entry.type === "compaction") { delete entry.firstKeptEntryId; entry.firstKeptEntryIndex = entry.summary === "old summary" ? 4 : -1; } }
	const inputs = [
		{ name: "v1", content: "not json\n" + jsonl(v1).trimEnd() },
		{ name: "v1-explicit", content: jsonl([{ ...v1[0], version: 1 }, ...v1.slice(1)]) },
		{ name: "v2-falsy-lines", content: 'null\nfalse\n0\n""\n' + jsonl(legacy) },
		{ name: "v2", content: jsonl(legacy).replace("\n", "\n{broken\n") },
		{ name: "pi-writer-v3", content: jsonl(all) },
		{ name: "v3-no-newline", content: jsonl(all).trimEnd() },
		{ name: "future-version", content: jsonl([{ ...all[0], version: 4 }, ...all.slice(1)]) },
		...['{"type":"session","id":null}', '{"type":"session","id":3}', '{"type":"message","id":"bad"}', 'not json\n', '  \n'].map((content, i) => ({ name: `invalid-${i}`, content })),
	];
	const results = inputs.map(({ name, content }) => {
		const path = join(dir, `${name}.jsonl`);
		writeFileSync(path, content);
		let manager;
		try { manager = SessionManager.open(path); } catch (error) { return { name, error: error.message.includes("not a valid pi session"), preserved: readFileSync(path, "utf8") === content }; }
		const migrated = name.startsWith("v1");
		// Capture context before append; v3 without a newline follows the baseline's literal append semantics.
		const context = manager.buildSessionContext();
		const entries = manager.getEntries();
		const ids = new Map(entries.map((e, i) => [e.id, `entry-${i}`]));
		if (migrated) for (const message of context.messages) if (ids.has(message.fromId)) message.fromId = ids.get(message.fromId);
		manager.appendMessage(user);
		const records = read(path);
		const last = records.at(-1);
		if (last?.message?.content === "continued") { last.id = "appended"; last.timestamp = timestamp; }
		return { name, llmContext: convertToLlm(context.messages), label: manager.getLabel(labelTarget) ?? null, sessionName: manager.getSessionName() ?? null, records: normalize(records, { generated: migrated, generatedTimestamps: false }), context, reopenedRoles: SessionManager.open(path).buildSessionContext().messages.map((m) => m.role) };
	});
	const input = { sessions: inputs, append: user, labelTarget, normalization: "Only generated entry IDs/references and generated timestamps; Pi writer header cwd. Stable historical IDs, fields, message timestamps and content are preserved." };
	const caseValue = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-interop", catalog_id: "contract:session/v3-jsonl", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
	const observation = { outcome: results, side_effects: [] };
	const fixture = { schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit, upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference }, case: caseValue, observation, input_hash: hash({ ...caseValue, input: canonical(input) }), observation_hash: hash({ outcome: canonical(results), side_effects: [] }), execution_method: "node --experimental-strip-types parity/oracle/session-interop.mjs <locked-pi-checkout>", platform: "any", environment: { oracle_entry: reference } };
	const output = join(root, "parity/oracle/fixtures/session-interop.json");
	if (check) {
		if (JSON.stringify(fixture) !== JSON.stringify(JSON.parse(readFileSync(output)))) throw Error("Session interop fixture drift");
	} else writeFileSync(output, JSON.stringify(fixture, null, 2) + "\n");
	const pigFile = join(dir, "pig-writer.jsonl");
	execFileSync("go", ["run", "./examples/session-interop", pigFile], { cwd: root });
	const pig = SessionManager.open(pigFile);
	const pigRecords = normalize(read(pigFile), { firstGenerated: 2 });
	const pigContext = pig.buildSessionContext();
	// The runtime user timestamp is generated by Pig. Message content and Assistant timestamp remain exact.
	for (const entry of pigRecords) if (entry.message?.role === "user" && entry.message.content !== "hi") entry.message.timestamp = 5;
	for (const message of pigContext.messages) if (message.role === "user" && message.content !== "hi") message.timestamp = 5;
	const llmContext = convertToLlm(pigContext.messages);
 for (const text of ["typed custom", "explicit null", "typed branch summary", "typed compaction summary"]) {
  if (!JSON.stringify(llmContext).includes(text)) throw Error(`Pi lost typed Pig message: ${text}`);
 }
 const reverse = { llmContext, baseline_commit: lock.upstream.commit, producer: "go run ./examples/session-interop <path>", consumer: `${reference}#SessionManager.open`, normalization: "Generated entry IDs/references, entry timestamps and runtime user timestamp only", records: pigRecords, context: pigContext };
	const reversePath = join(root, "parity/oracle/fixtures/session-interop-pig-writer.json");
	if (check) {
		if (JSON.stringify(canonical(reverse)) !== JSON.stringify(canonical(JSON.parse(readFileSync(reversePath))))) throw Error("Pig writer / Pi reader fixture drift");
	} else writeFileSync(reversePath, JSON.stringify(reverse, null, 2) + "\n");
	console.log(`${check ? "verified" : "wrote"} ${output}`);
} finally { rmSync(dir, { recursive: true, force: true }); }
