import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const pi = process.argv[2];
const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json")));
if (execFileSync("git", ["-C", pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim() !== lock.upstream.commit) throw Error("wrong Pi baseline");
if (execFileSync("git", ["-C", pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" })) throw Error("dirty Pi baseline");
const reference = "packages/coding-agent/src/core/sdk.ts";
const load = (path) => import(pathToFileURL(join(pi, path)));
const { createAgentSession } = await load(reference);
const { SessionManager } = await load("packages/coding-agent/src/core/session-manager.ts");
const { SettingsManager } = await load("packages/coding-agent/src/core/settings-manager.ts");
const { ModelRuntime } = await load("packages/coding-agent/src/core/model-runtime.ts");
const { AuthStorage } = await load("packages/coding-agent/src/core/auth-storage.ts");
const { createFauxCore, fauxAssistantMessage } = await load("packages/ai/src/providers/faux.ts");
const text = (m) => typeof m.content === "string" ? m.content : m.content.filter(c => c.type === "text").map(c => c.text).join("");
const canonical = (v) => Array.isArray(v) ? v.map(canonical) : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([k, v]) => [k, canonical(v)])) : v;
const hash = (v) => `sha256:${createHash("sha256").update(JSON.stringify(v)).digest("hex")}`;
const dir = mkdtempSync(join(tmpdir(), "pi-session-messages-"));
const input = { modes: ["one-at-a-time", "all"], prompt: [{ type: "text", text: "first" }, { type: "text", text: "second" }], steering: ["s1", "s2"], followUp: ["f1", "f2"], reply: "reply" };
const outcomes = [];
try {
 for (const steeringMode of input.modes) for (const followUpMode of input.modes) {
  const core = createFauxCore({ tokenSize: { min: 100, max: 100 } });
  core.setResponses(Array(6).fill(fauxAssistantMessage(input.reply, { timestamp: 2 })));
  const model = { ...core.getModel(), provider: "anthropic" };
  const runtime = await ModelRuntime.create({ credentials: AuthStorage.inMemory({ [model.provider]: { type: "api_key", key: "fixture" } }), modelsPath: null, allowModelNetwork: false });
  const manager = SessionManager.create(dir, dir);
  const settings = SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } });
  const { session } = await createAgentSession({ cwd: dir, agentDir: dir, model, modelRuntime: runtime, sessionManager: manager, settingsManager: settings, noTools: "all" });
  const requests = [], queues = [], events = [];
  session.setSteeringMode(steeringMode);
  session.setFollowUpMode(followUpMode);
  let rejected = false;
  runtime.streamSimple = async (m, context, options) => {
   requests.push(context.messages.map(m => `${m.role}:${text(m)}`));
   if (requests.length === 1) {
    await session.steer("cleared"); await session.followUp("cleared"); session.clearQueue();
    await session.steer(input.steering[0]);
    await session.sendUserMessage(input.steering[1], { deliverAs: "steer" });
    await session.followUp(input.followUp[0]);
    await session.prompt(input.followUp[1], { streamingBehavior: "followUp" });
    try { await session.sendUserMessage("rejected"); } catch { rejected = true; }
   }
   return core.streamSimple(m, context, options);
  };
  session.subscribe(e => {
   if (e.type === "queue_update") { queues.push({ steering: e.steering, followUp: e.followUp }); events.push(`queue:${e.steering.length}:${e.followUp.length}`); }
   if (e.type === "message_start" && e.message.role === "user") events.push(`start:${text(e.message)}`);
   if (e.type === "agent_settled") events.push("settled");
  });
  await session.sendUserMessage(input.prompt, { deliverAs: "followUp" });
  const history = session.messages.map(m => `${m.role}:${text(m)}`);
  const reopened = SessionManager.open(manager.getSessionFile()).buildSessionContext().messages.map(m => `${m.role}:${text(m)}`);
  outcomes.push({ steeringMode, followUpMode, requests, queues, events, history, reopened, rejected, pending: session.pendingMessageCount, settings: [settings.getSteeringMode(), settings.getFollowUpMode()] });
  session.dispose();
 }
 const admission = [];
 for (const phase of ["idle", "cancelled", "agent_end", "agent_settled"]) {
  const core = createFauxCore({ tokenSize: { min: 100, max: 100 } });
  core.setResponses([fauxAssistantMessage("reply", { timestamp: 2 })]);
  const model = { ...core.getModel(), provider: "anthropic" };
  const runtime = await ModelRuntime.create({ credentials: AuthStorage.inMemory({ anthropic: { type: "api_key", key: "fixture" } }), modelsPath: null, allowModelNetwork: false });
  const { session } = await createAgentSession({ cwd: dir, agentDir: dir, model, modelRuntime: runtime, sessionManager: SessionManager.inMemory(dir), settingsManager: SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false } }), noTools: "all" });
  const accepted = [];
  const enqueue = () => Promise.all([session.steer("late-steer").then(() => accepted.push("steer")), session.followUp("late-follow").then(() => accepted.push("followUp"))]);
  let enqueued, aborted;
  runtime.streamSimple = async (m, context, options) => {
   return core.streamSimple(m, context, options);
  };
  session.subscribe(e => {
   if (phase === "cancelled" && e.type === "message_update" && !aborted) { aborted = session.abort(); enqueued = enqueue(); }
   if (e.type === phase && !enqueued) enqueued = enqueue();
  });
  if (phase === "idle") await enqueue(); else await session.sendUserMessage("start");
  await enqueued; await aborted;
  admission.push({ phase, accepted, steering: session.getSteeringMessages(), followUp: session.getFollowUpMessages() });
  session.dispose();
 }
 const c = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-messages", catalog_id: "contract:codingagent/session-messages", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
 const observation = { outcome: outcomes, side_effects: [] };
 const fixture = { schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit, upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference }, case: c, observation, input_hash: hash({ ...c, input: canonical(input) }), observation_hash: hash({ outcome: canonical(outcomes), side_effects: [] }), execution_method: "node --experimental-strip-types parity/oracle/session-messages.mjs <locked-pi-checkout>", platform: "any", environment: { oracle_entry: reference } };
 const out = join(root, "parity/oracle/fixtures/session-messages.json");
 if (process.argv.includes("--check")) { if (JSON.stringify(fixture) !== JSON.stringify(JSON.parse(readFileSync(out)))) throw Error("fixture drift"); }
 else writeFileSync(out, JSON.stringify(fixture, null, 2) + "\n");
 const deviationCase = { ...c, id: "go-sdk/codingagent/session-messages-deviation", input: { phases: ["idle", "cancelled", "agent_end", "agent_settled"] } };
 const deviationObservation = { outcome: admission, side_effects: [] };
 const deviation = { ...fixture, case: deviationCase, observation: deviationObservation, input_hash: hash({ ...deviationCase, input: canonical(deviationCase.input) }), observation_hash: hash({ outcome: canonical(admission), side_effects: [] }) };
 const deviationOut = out.replace(".json", "-deviation.json");
 if (process.argv.includes("--check")) { if (JSON.stringify(deviation) !== JSON.stringify(JSON.parse(readFileSync(deviationOut)))) throw Error("admission fixture drift"); }
 else writeFileSync(deviationOut, JSON.stringify(deviation, null, 2) + "\n");
 console.log(`verified ${out}`);
} finally { rmSync(dir, { recursive: true, force: true }); }
