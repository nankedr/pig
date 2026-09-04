import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "telemetry.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, child]) => [key, canonical(child)]));
	}
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
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value);
	if (Array.isArray(value)) return `[${value.map(canonicalJSONString).join(",")}]`;
	if (typeof value === "object") return `{${Object.entries(value).map(([key, child]) => `${JSON.stringify(key)}:${canonicalJSONString(child)}`).join(",")}}`;
	throw new Error(`unsupported JSON value: ${typeof value}`);
}

function digest(value) {
	return `sha256:${createHash("sha256").update(canonicalJSONString(canonical(value))).digest("hex")}`;
}

function caseDigest(value) {
	return `sha256:${createHash("sha256").update(canonicalJSONString({
		schema_version: value.schema_version, id: value.id, catalog_id: value.catalog_id, surface: value.surface,
		input: canonical(value.input), observe: value.observe,
	})).digest("hex")}`;
}

function parseArgs(argv) {
	const result = { pi: defaultPi, out: defaultOutput, check: false };
	const args = argv.slice(2);
	for (let i = 0; i < args.length; i++) {
		if (args[i] === "--check") result.check = true;
		else if (args[i] === "--out") result.out = args[++i];
		else if (result.pi === defaultPi) result.pi = args[i];
		else throw new Error(`unexpected argument: ${args[i]}`);
	}
	return result;
}

async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
 const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
 if (head !== lock.upstream.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.upstream.commit}`);
 const status = spawnSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
 if (status.status !== 0 || status.stdout !== "") throw new Error("locked Pi checkout has tracked changes");
 const reference = "packages/telemetry/src/memory.ts";
 const { InMemoryTelemetryContext } = await import(pathToFileURL(join(args.pi, reference)).href);
 const { createTelemetryAdapterConformance } = await import(pathToFileURL(join(args.pi, "packages/telemetry/src/testing/conformance.ts")).href);
 const input = { attributes: { values: ["initial"], count: 0 }, failure: "failed", projection: "Detached span snapshots, callback identity, start/end order and nine public conformance results; Go errors use Error as name, invalid dynamic values replace unreadable JS proxies." };
 const caseValue = { schema_version: "1.0.0", id: "go-sdk/telemetry/memory", catalog_id: "contract:telemetry/memory", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
 const memory = new InMemoryTelemetryContext();
 const attributes = structuredClone(input.attributes);
 let captured, active;
 const expected = { value: 42 };
 const result = await memory.startSpan({ name: "parent", attributes }, async (parent) => {
  captured = parent;
  active = memory.getSpans();
  attributes.values[0] = "changed";
  parent.setAttributes({ count: 1, ignored: undefined });
  parent.addEvent("first", { values: attributes.values });
  attributes.values[0] = "changed-again";
  let admit, release;
  const started = new Promise((resolve) => { admit = resolve; });
  const gate = new Promise((resolve) => { release = resolve; });
  const first = parent.startSpan({ name: "first-child" }, async () => { admit(); await gate; });
  await started;
  await parent.startSpan({ name: "second-child" }, (span) => { span.setStatus({ status: "error", error: { name: "Expected", message: "handled" } }); });
  release();
  await first;
  parent.addEvent("second");
  return expected;
 });
 const failure = new Error(input.failure);
 let sameError = false;
 try { await memory.startSpan({ name: "failure" }, () => { throw failure; }); } catch (error) { sameError = error === failure; }
 try { await memory.startSpan({ name: "explicit" }, (span) => { span.setStatus({ status: "ok" }); throw failure; }); } catch (error) { if (error !== failure) throw error; }
 const detached = memory.getSpans();
 detached[0].attributes.values[0] = "snapshot-mutation";
 detached[0].events[0].attributes.values[0] = "snapshot-mutation";
 detached[2].status.error.message = "snapshot-mutation";
 captured.setAttributes({ late: true });
 captured.addEvent("late");
 const late = await captured.startSpan({ name: "late-child" }, () => 7);
 const conformance = [];
 for (const c of createTelemetryAdapterConformance(async () => {
  const context = new InMemoryTelemetryContext();
  return { context, getSpans: async () => context.getSpans(), [Symbol.asyncDispose]: async () => {} };
 })) { await c.run(); conformance.push({ group: c.group, name: c.name, passed: true }); }
 const observation = { outcome: { active, spans: memory.getSpans(), same_result: result === expected, same_error: sameError, late_result: late, conformance }, side_effects: [] };
 const fixture = {
  schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
  upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: `${reference}#InMemoryTelemetryContext` },
  case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation),
  execution_method: "node --experimental-strip-types parity/oracle/telemetry.mjs <locked-pi-checkout>", platform: "any",
  environment: { node: process.version, oracle_entry: reference },
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
}
main().catch((error) => { console.error(`oracle: ${error.message}`); process.exit(1); });
