import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const checkout = process.argv.slice(2).find(a => !a.startsWith("--")) ?? join(root, ".upstream", "pi");
const path = join(root, "parity/oracle/fixtures/responses-tools.json");
const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json")));
const commit = execFileSync("git", ["-C", checkout, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
if (commit !== lock.upstream.commit) throw new Error("checkout differs from fixed Pi baseline");
if (execFileSync("git", ["-C", checkout, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" }).trim()) throw new Error("Pi checkout has tracked changes");
const { stream } = await import(pathToFileURL(join(checkout, "packages/ai/src/api/openai-responses.ts")).href);
const model = { id: "fixture", name: "Fixture", api: "openai-responses", provider: "deepseek", baseUrl: "https://local.invalid", reasoning: false, input: ["text"], cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }, contextWindow: 32000, maxTokens: 512 };
const user = { role: "user", content: "lookup", timestamp: 1 };
const tools = [{ name: "lookup", description: "Look up a value", parameters: { type: "object", properties: { query: { type: "string" } }, required: ["query"] } }];
const call = (id, args) => ({ type: "function_call", id: `fc_${id}`, call_id: id, name: "lookup", arguments: args });
const cases = [];
for (const name of ["single", "batch", "arguments done suffix", "partial truncated", "partial length", "history replay", "choice none", "choice function"]) {
  const context = { messages: [user], tools };
  if (name === "history replay") {
    context.messages.push({ role: "assistant", api: model.api, provider: model.provider, model: model.id, stopReason: "toolUse", timestamp: 2, usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } }, content: [{ type: "toolCall", id: "a|fc_a", name: "lookup", arguments: { query: "hello" } }] });
    context.messages.push({ role: "toolResult", toolCallId: "a|fc_a", toolName: "lookup", content: [{ type: "text", text: "result" }], isError: false, timestamp: 3 });
  }
  const events = [];
  if (name !== "history replay" && !name.startsWith("choice")) {
    for (const [index, id] of (name === "batch" ? ["a", "b"] : ["a"]).entries()) {
      events.push({ type: "response.output_item.added", output_index: index, item: call(id, "") });
      const fragments = name === "arguments done suffix" || name === "partial truncated" || name === "partial length" ? ['{"query":"hel'] : ['{"query":', '"hello"}'];
      for (const delta of fragments) events.push({ type: "response.function_call_arguments.delta", output_index: index, delta });
      if (name === "partial length") {
        events.push({ type: "response.function_call_arguments.done", output_index: index, arguments: '{"query":"hel' });
        events.push({ type: "response.output_item.done", output_index: index, item: { ...call(id, '{"query":"hel'), status: "incomplete" } });
      } else if (name !== "partial truncated") {
        events.push({ type: "response.function_call_arguments.done", output_index: index, arguments: '{"query":"hello"}' });
        events.push({ type: "response.output_item.done", output_index: index, item: call(id, '{"query":"hello"}') });
      }
    }
  }
  if (name === "partial length") events.push({ type: "response.incomplete", response: { status: "incomplete", incomplete_details: { reason: "max_output_tokens" }, output: [] } });
  else if (name !== "partial truncated") events.push({ type: "response.completed", response: { status: "completed", output: [] } });
  const sse = events.map(e => `data: ${JSON.stringify(e)}\n\n`).join("");
  const options = name === "choice none" ? { toolChoice: "none" } : name === "choice function" ? { toolChoice: { type: "function", name: "lookup" } } : {};
  let request;
  const result = stream(model, context, { apiKey: "fixture-key", maxTokens: 512, cacheRetention: "none", ...options, fetch: async (url, init) => {
    request = { url: String(url), body: JSON.parse(init.body) };
    return new Response(sse, { headers: { "content-type": "text/event-stream" } });
  }});
  const types = [];
  for await (const event of result) types.push(event.type);
  const out = await result.result();
  const outcome = { content: out.content.map(c => ({ type: c.type, id: c.id, name: c.name, arguments: c.arguments })), stopReason: out.stopReason };
  cases.push({ name, context, options, sse, request, types, outcome });
}
const hash = v => createHash("sha256").update(JSON.stringify(v)).digest("hex");
const fixture = { schema_version: "1.0.0", baseline_commit: commit, deterministic: true, source: "packages/ai/src/api/openai-responses.ts#stream", observation_scope: "function tools, call_id|item_id, arguments delta/done suffix, multiple calls, truncated partial, tool history and choices; DeepSeek plain reasoning and strict protocol validation are separate service conformance", model, cases, observation_hash: hash(cases) };
if (process.argv.includes("--check")) {
  if (JSON.stringify(JSON.parse(readFileSync(path))) !== JSON.stringify(fixture)) throw new Error("Responses tools Oracle fixture drift");
  console.error("Responses tools fixed Pi fixture reproduces");
} else {
  writeFileSync(path, JSON.stringify(fixture, null, 2) + "\n");
  console.error("Responses tools fixed Pi fixture captured");
}
