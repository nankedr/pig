import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const checkout = process.argv.slice(2).find(a => !a.startsWith("--")) ?? join(root, ".upstream", "pi");
const path = join(root, "parity", "oracle", "fixtures", "responses-text.json");
const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json")));
const commit = execFileSync("git", ["-C", checkout, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
if (commit !== lock.upstream.commit) throw new Error("checkout differs from fixed Pi baseline");
const { stream, streamSimple } = await import(pathToFileURL(join(checkout, "packages", "ai", "src", "api", "openai-responses.ts")).href);
const model = { id: "configured-text-model", name: "Fixture", api: "openai-responses", provider: "deepseek", baseUrl: "https://local.invalid", reasoning: false, input: ["text"], cost: { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 1.5 }, contextWindow: 32000, maxTokens: 512 };
const context = { messages: [{ role: "user", content: "你好", timestamp: 1 }] };
const item = { type: "message", id: "msg-fixture", role: "assistant", status: "completed", content: [{ type: "output_text", text: "你好🙂", annotations: [] }] };
const usage = { input_tokens: 10, output_tokens: 5, total_tokens: 15, input_tokens_details: { cached_tokens: 3, cache_write_tokens: 1 }, output_tokens_details: { reasoning_tokens: 2 } };
const prefix = [
  { type: "response.created", response: { id: "resp-fixture", status: "in_progress" } },
  { type: "response.output_item.added", output_index: 0, item: { ...item, status: "in_progress", content: [] } },
  { type: "response.output_text.delta", output_index: 0, content_index: 0, delta: "你" },
  { type: "response.output_text.delta", output_index: 0, content_index: 0, delta: "好🙂" },
  { type: "response.output_item.done", output_index: 0, item },
];
const cases = [];
for (const [name, status, reason, simple, options = {}] of [["completed", "completed"], ["length", "incomplete", "max_output_tokens"], ["filtered", "incomplete", "content_filter"], ["failed", "failed"], ["truncated", null], ["simple deferred", "completed", null, true, { deferred: true }], ["simple deferred window", "completed", null, true, { deferred: { window: "1h" } }], ["simple thinking budgets", "completed", null, true, { thinkingBudgets: { high: 8192 } }], ["cache none session", "completed", null, false, { sessionId: "fixture-session" }]]) {
  const events = [...prefix];
  if (status) events.push({ type: `response.${status}`, response: { id: "resp-fixture", status, output: [item], ...(status !== "failed" ? { usage } : {}), ...(reason ? { incomplete_details: { reason } } : {}), ...(status === "failed" ? { error: { code: "bad", message: "fixture failure" } } : {}) } });
  const sse = events.map(e => `data: ${JSON.stringify(e)}\n\n`).join("");
  let request;
  const result = (simple ? streamSimple : stream)(model, context, { apiKey: "fixture-key", maxTokens: 512, cacheRetention: "none", ...options, fetch: async (url, init) => {
    request = { url: String(url), body: JSON.parse(init.body) };
    return new Response(sse, { headers: { "content-type": "text/event-stream" } });
  }});
  const types = [];
  for await (const event of result) types.push(event.type);
  const out = await result.result();
  const outcome = { content: out.content.map(c => ({ type: c.type, text: c.text })), stopReason: out.stopReason, rawStopReason: out.rawStopReason ?? null, errorMessage: out.errorMessage ?? null, usage: out.usage };
  cases.push({ name, simple: !!simple, options, sse, request, types, outcome });
}
const input = { model, context };
const hash = value => createHash("sha256").update(JSON.stringify(value)).digest("hex");
const fixture = { schema_version: "1.0.0", baseline_id: lock.baseline_id, baseline_commit: commit, deterministic: true, source: "packages/ai/src/api/openai-responses.ts#stream", observation_scope: "single text output item, terminal state/error, usage/cost and event types; Simple deferred/thinkingBudgets and cache none sessionId no-ops; request has no system prompt; textSignature and timestamps are outside this observation", input, input_hash: hash(input), cases, observation_hash: hash(cases) };
if (process.argv.includes("--check")) {
  const saved = JSON.parse(readFileSync(path));
  if (JSON.stringify(saved) !== JSON.stringify(fixture)) throw new Error("Responses Oracle fixture drift");
  console.error("Responses fixed Pi fixture reproduces");
} else {
  writeFileSync(path, JSON.stringify(fixture, null, 2) + "\n");
  console.error("Responses fixed Pi fixture captured");
}
