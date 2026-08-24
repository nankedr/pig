// Captures the deterministic M0 no-op cases for Pi's OpenAI Chat Completions
// adapter. The harness runs only against the locked Pi checkout and replaces
// network I/O with an in-memory fetch implementation. Normal Pig tests replay
// the committed evidence without Node or third-party dependencies.
//
// Usage:
//   node --experimental-strip-types openai-completions-m0-no-op.mjs [<pi-root>] [--out <path>]
//   node --experimental-strip-types openai-completions-m0-no-op.mjs --check [<pi-root>]

// The pinned Pi checkout needs its packages/ai production dependencies
// installed (npm install --ignore-scripts --omit=dev --no-package-lock
// --workspaces=false). The injected fetch guarantees that capture/check never
// contacts a provider.

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const SCHEMA_VERSION = "1.0.0";
const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-m0-no-op.json");

const cases = [
  {
    id: "metadata",
    catalog_id: "matrix:ai/openai-completions/option/open-aicompletions-options-metadata",
    entrypoint: "stream",
    variants: [
      { id: "empty", option: { metadata: {} } },
      { id: "value", option: { metadata: { trace: { enabled: true } } } },
    ],
  },
  {
    id: "telemetry-context",
    catalog_id: "matrix:ai/openai-completions/option/open-aicompletions-options-telemetry-context",
    entrypoint: "stream",
    variants: [{ id: "recording-context", option: { telemetryContext: "recording-context" } }],
  },
  {
    id: "transport",
    catalog_id: "matrix:ai/openai-completions/option/open-aicompletions-options-transport",
    entrypoint: "stream",
    variants: ["sse", "websocket", "websocket-cached", "auto"].map((transport) => ({
      id: transport,
      option: { transport },
    })),
  },
  {
    id: "websocket-connect-timeout-ms",
    catalog_id:
      "matrix:ai/openai-completions/option/open-aicompletions-options-websocket-connect-timeout-ms",
    entrypoint: "stream",
    variants: [
      { id: "zero", option: { websocketConnectTimeoutMs: 0 } },
      { id: "value", option: { websocketConnectTimeoutMs: 17 } },
    ],
  },
  {
    id: "deferred",
    catalog_id: "matrix:ai/openai-completions/option/simple-stream-options-deferred",
    entrypoint: "streamSimple",
    variants: [
      { id: "false", option: { deferred: false } },
      { id: "true", option: { deferred: true } },
      { id: "empty", option: { deferred: {} } },
    ],
  },
  {
    id: "deferred-window",
    catalog_id: "matrix:ai/openai-completions/option/simple-stream-options-deferred-window",
    entrypoint: "streamSimple",
    variants: [
      { id: "15m", option: { deferred: { window: "15m" } } },
      { id: "1h", option: { deferred: { window: "1h" } } },
      { id: "24h", option: { deferred: { window: "24h" } } },
    ],
  },
];

function parseArgs(argv) {
  const parsed = { piRoot: null, out: defaultOutput, check: false };
  const rest = argv.slice(2);
  for (let i = 0; i < rest.length; i++) {
    if (rest[i] === "--check") parsed.check = true;
    else if (rest[i] === "--out") parsed.out = rest[++i];
    else if (!parsed.piRoot) parsed.piRoot = rest[i];
    else throw new Error(`unexpected argument: ${rest[i]}`);
  }
  parsed.piRoot ??= defaultPiRoot;
  return parsed;
}

function readLock() {
  const lock = JSON.parse(readFileSync(join(repoRoot, "parity", "baseline", "upstream.lock.json"), "utf8"));
  const commit = lock?.source_verification?.expected_commit ?? lock?.upstream?.commit;
  if (!commit || !lock?.baseline_id || !lock?.upstream?.repository) throw new Error("baseline lock is incomplete");
  return { commit, baselineID: lock.baseline_id, repository: lock.upstream.repository };
}

function assertCheckoutAtLock(piRoot, expectedCommit) {
  const head = execFileSync("git", ["-C", piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
  if (head !== expectedCommit) {
    throw new Error(`checkout HEAD ${head} != locked commit ${expectedCommit}`);
  }
}

function canonical(value) {
  if (Array.isArray(value)) return value.map(canonical);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value)
        .filter(([key]) => key !== "timestamp")
        .sort(([left], [right]) => left.localeCompare(right))
        .map(([key, child]) => [key, canonical(child)]),
    );
  }
  return value;
}

function stableJSON(value) {
  return JSON.stringify(canonical(value));
}

function sha256(value) {
  return createHash("sha256").update(typeof value === "string" ? value : stableJSON(value)).digest("hex");
}

function normalizedHeaders(headers) {
  return Object.fromEntries(
    [...new Headers(headers).entries()]
      .filter(([name]) => name !== "user-agent" && name !== "content-length" && !name.startsWith("x-stainless-"))
      .sort(([left], [right]) => left.localeCompare(right)),
  );
}

function normalizedRequest(input, init = {}) {
  const body = typeof init.body === "string" ? JSON.parse(init.body) : init.body ?? null;
  return canonical({
    url: String(input),
    method: init.method ?? "GET",
    headers: normalizedHeaders(init.headers),
    body,
  });
}

const sseBody = [
  'data: {"id":"chatcmpl-oracle","model":"oracle-model","choices":[{"index":0,"delta":{"role":"assistant","content":"pong"},"finish_reason":null}]}',
  'data: {"id":"chatcmpl-oracle","model":"oracle-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}',
  "data: [DONE]",
  "",
].join("\n\n");

const model = {
  id: "oracle-model",
  name: "Oracle Model",
  api: "openai-completions",
  provider: "oracle",
  baseUrl: "https://oracle.invalid/v1",
  reasoning: false,
  input: ["text"],
  cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
  contextWindow: 8192,
  maxTokens: 1024,
  compat: { supportsUsageInStreaming: true, maxTokensField: "max_tokens" },
};
const context = { messages: [{ role: "user", content: "ping", timestamp: 0 }] };

function materializeOption(option, telemetry) {
  if (option.telemetryContext !== "recording-context") return option;
  return {
    ...option,
    telemetryContext: {
      async startSpan() {
        telemetry.starts++;
        throw new Error("unexpected telemetry span");
      },
    },
  };
}

async function runInvocation(api, entrypoint, option) {
  const requests = [];
  const telemetry = { starts: 0 };
  const fetch = async (input, init) => {
    requests.push(normalizedRequest(input, init));
    return new Response(sseBody, { status: 200, headers: { "content-type": "text/event-stream" } });
  };
  const stream = api[entrypoint](model, context, {
    apiKey: "fixture-key",
    fetch,
    ...materializeOption(option, telemetry),
  });
  const events = [];
  for await (const event of stream) events.push(canonical(event));
  const outcome = canonical(await stream.result());
  return {
    request_count: requests.length,
    request_sha256: sha256(requests),
    events_sha256: sha256(events),
    outcome_sha256: sha256(outcome),
    telemetry_start_count: telemetry.starts,
  };
}

async function captureCase(api, definition) {
  const control = await runInvocation(api, definition.entrypoint, {});
  const observations = [];
  for (const variant of definition.variants) {
    const observed = await runInvocation(api, definition.entrypoint, variant.option);
    observations.push({
      variant: variant.id,
      ...observed,
      request_equal: observed.request_sha256 === control.request_sha256,
      events_equal: observed.events_sha256 === control.events_sha256,
      outcome_equal: observed.outcome_sha256 === control.outcome_sha256,
    });
  }
  const input = { entrypoint: definition.entrypoint, variants: definition.variants };
  return {
    id: definition.id,
    catalog_id: definition.catalog_id,
    input,
    input_sha256: sha256(input),
    expected: {
      comparison: "each variant equals the same entrypoint with the option absent",
      request_count: 1,
      request_equal: true,
      events_equal: true,
      outcome_equal: true,
      telemetry_start_count: 0,
    },
    actual: { control, observations },
  };
}

function assertExpected(fixtureCases) {
  for (const captured of fixtureCases) {
    if (captured.actual.control.request_count !== captured.expected.request_count) {
      throw new Error(`${captured.id}: control request count mismatch`);
    }
    for (const observed of captured.actual.observations) {
      for (const key of ["request_count", "request_equal", "events_equal", "outcome_equal", "telemetry_start_count"]) {
        if (observed[key] !== captured.expected[key]) {
          throw new Error(`${captured.id}/${observed.variant}: ${key}=${observed[key]} want ${captured.expected[key]}`);
        }
      }
    }
  }
}

async function buildFixture(piRoot, lock) {
  const modulePath = join(piRoot, "packages", "ai", "src", "api", "openai-completions.ts");
  const api = await import(pathToFileURL(modulePath).href);
  const capturedCases = [];
  for (const definition of cases) capturedCases.push(await captureCase(api, definition));
  assertExpected(capturedCases);
  const observationsSHA256 = sha256(capturedCases.map(({ id, catalog_id, input_sha256, expected, actual }) => ({
    id, catalog_id, input_sha256, expected, actual,
  })));
  return {
    schema_version: SCHEMA_VERSION,
    id: "ai/openai-completions/m0-no-op",
    catalog_ids: cases.map((entry) => entry.catalog_id),
    baseline_id: lock.baselineID,
    baseline_commit: lock.commit,
    deterministic: true,
    upstream: {
      module: "ai",
      repository: lock.repository,
      commit: lock.commit,
      reference: "packages/ai/src/api/openai-completions.ts#stream,streamSimple",
    },
    normalizations: [
      "remove AssistantMessage.timestamp recursively",
      "remove user-agent, content-length, and x-stainless-* request headers",
      "sort object keys recursively before hashing",
    ],
    cases: capturedCases,
    hash: { algorithm: "sha256", observations_sha256: observationsSHA256 },
    env: { node_version: process.version, platform: process.platform, arch: process.arch },
    exec_method: "node --experimental-strip-types parity/oracle/openai-completions-m0-no-op.mjs",
  };
}

function fixtureFacts(fixture) {
  return stableJSON({
    schema_version: fixture.schema_version,
    id: fixture.id,
    catalog_ids: fixture.catalog_ids,
    baseline_id: fixture.baseline_id,
    baseline_commit: fixture.baseline_commit,
    deterministic: fixture.deterministic,
    upstream: fixture.upstream,
    normalizations: fixture.normalizations,
    cases: fixture.cases,
    hash: fixture.hash,
    exec_method: fixture.exec_method,
  });
}

async function main() {
  const args = parseArgs(process.argv);
  const lock = readLock();
  assertCheckoutAtLock(args.piRoot, lock.commit);
  const fixture = await buildFixture(args.piRoot, lock);
  if (args.check) {
    const committed = JSON.parse(readFileSync(args.out, "utf8"));
    if (fixtureFacts(committed) !== fixtureFacts(fixture)) {
      throw new Error("committed M0 no-op fixture does not reproduce");
    }
    console.error(`oracle: committed fixture reproduces (${fixture.cases.length} option groups)`);
    return;
  }
  mkdirSync(dirname(args.out), { recursive: true });
  writeFileSync(args.out, JSON.stringify(fixture, null, 2) + "\n");
  console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
  console.error("oracle:", error.message);
  process.exit(1);
});
