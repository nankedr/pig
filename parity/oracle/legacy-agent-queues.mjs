import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "legacy-agent-queues.json");

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
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
 if (head !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" }) !== "") throw new Error("Pi checkout has tracked changes");
 const reference = "packages/agent/src/agent.ts";
 const { Agent } = await import(pathToFileURL(join(args.pi, reference)).href);
 const { createFauxCore, fauxAssistantMessage } = await import(pathToFileURL(join(args.pi, "packages/ai/src/providers/faux.ts")).href);
 const user = (text) => ({ role: "user", content: text, timestamp: 1 });
 const input = { modes: ["one-at-a-time", "all"], resume: [false, true], prompt: "prompt", steering: ["s1", "s2"], follow_up: ["f1", "f2"], reply: "reply" };
 const outcomes = [];
 for (const steeringMode of input.modes) for (const followUpMode of input.modes) for (const resume of input.resume) {
  const core = createFauxCore({ tokenSize: { min: 100, max: 100 } });
  core.setResponses(Array(5).fill(fauxAssistantMessage(input.reply, { timestamp: 2 })));
  const requests = [], events = [], listeners = [];
  const agent = new Agent({ steeringMode, followUpMode,
   shouldStopAfterTurn: () => resume && requests.length === 1,
   streamFn: (model, context, options) => {
    const users = [];
    for (let i = context.messages.length - 1; i >= 0 && context.messages[i].role === "user"; i--) users.unshift(context.messages[i].content);
    requests.push(users);
    if (requests.length === 1) {
     input.steering.forEach((text) => agent.steer(user(text)));
     input.follow_up.forEach((text) => agent.followUp(user(text)));
    }
    return core.streamSimple(model, context, options);
   },
  });
  agent.subscribe(async (event) => {
   if (["agent_start", "turn_start", "turn_end", "agent_end"].includes(event.type)) events.push(event.type);
   if (event.type === "message_end") events.push(event.message.role === "user" ? `user:${event.message.content}` : "assistant");
   if (event.type === "agent_end") { listeners.push("first-start"); await Promise.resolve(); listeners.push("first-end"); }
  });
  agent.subscribe((event) => { if (event.type === "agent_end") listeners.push("second"); });
  await agent.prompt(user(input.prompt));
  if (resume) await agent.continue();
  await agent.waitForIdle();
  outcomes.push({ steeringMode, followUpMode, resume, requests, events, listeners, queued: agent.hasQueuedMessages(), streaming: agent.state.isStreaming });
 }
 const idle = new Agent({ streamFn: () => { throw new Error("unexpected idle stream"); } });
 idle.steer(user("idle-steer"));
 idle.followUp(user("idle-follow-up"));
 const idleAccepted = idle.hasQueuedMessages();
 idle.reset();
 const core = createFauxCore({ tokenSize: { min: 100, max: 100 } });
 core.setResponses([fauxAssistantMessage("reply")]);
 const ending = new Agent({ streamFn: core.streamSimple });
 ending.subscribe((event) => { if (event.type === "agent_end") ending.followUp(user("late")); });
 await ending.prompt(user("prompt"));
 const errorCore = createFauxCore({ tokenSize: { min: 100, max: 100 } });
 errorCore.setResponses([fauxAssistantMessage("reply")]);
 const failing = new Agent({ streamFn: errorCore.streamSimple });
 const errorListeners = [];
 failing.subscribe((event) => { if (event.type === "agent_end") { errorListeners.push("first"); throw new Error("listener failed"); } });
 failing.subscribe((event) => { if (event.type === "agent_end") errorListeners.push("second"); });
 try { await failing.prompt(user("prompt")); } catch (error) { if (error.message !== "listener failed") throw error; }
 const cases = [
  { name: "legacy-agent-queues", id: "go-sdk/agent/legacy-queues", input, outcome: outcomes },
  { name: "legacy-agent-queues-deviation", id: "go-sdk/agent/legacy-queues-deviation", input: { operations: ["idle-enqueue", "end-listener-enqueue", "end-listener-error"] }, outcome: { idleAccepted, lateQueued: ending.hasQueuedMessages(), errorListeners } },
 ];
 for (const c of cases) {
  const declaration = { schema_version: "1.0.0", id: c.id, catalog_id: "contract:agent/legacy-queues", surface: "go-sdk", input: c.input, observe: ["outcome", "side_effects"] };
  const observation = { outcome: c.outcome, side_effects: [] };
  const fixture = {
   schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
   upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: `${reference}#Agent` },
   case: declaration, observation, input_hash: caseDigest(declaration), observation_hash: digest(observation),
   execution_method: "node --experimental-strip-types parity/oracle/legacy-agent-queues.mjs <locked-pi-checkout>", platform: "any",
   environment: { node: process.version, oracle_entry: reference },
  };
  const out = c.name.endsWith("deviation") ? args.out.replace(/\.json$/, "-deviation.json") : args.out;
  if (args.check) {
   const committed = JSON.parse(readFileSync(out, "utf8"));
   fixture.environment.node = committed.environment.node;
   if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error(`fixture does not reproduce: ${out}`);
   console.log(`verified ${out}`);
  } else { writeFileSync(out, `${JSON.stringify(fixture, null, 2)}\n`); console.log(`wrote ${out}`); }
 }
}
main().catch((error) => { console.error(error); process.exit(1); });
