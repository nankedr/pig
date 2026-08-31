import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-retry.json");
const fixedNow = Date.parse("2026-08-31T00:00:00Z");
const catalogIDs = [
	"matrix:ai/openai-completions/error/retry-budget-exhausted",
	"matrix:ai/openai-completions/error/retry-delay-cap-configured",
	"matrix:ai/openai-completions/error/retry-delay-cap-default",
	"matrix:ai/openai-completions/error/retry-delay-cap-disabled",
	"matrix:ai/openai-completions/error/retry-delay-exponential-jitter",
	"matrix:ai/openai-completions/error/retry-delay-retry-after-date",
	"matrix:ai/openai-completions/error/retry-delay-retry-after-ms",
	"matrix:ai/openai-completions/error/retry-delay-retry-after-ms-invalid",
	"matrix:ai/openai-completions/error/retry-delay-retry-after-seconds",
	"matrix:ai/openai-completions/error/retry-header-x-should-retry-false",
	"matrix:ai/openai-completions/error/retry-header-x-should-retry-true",
	"matrix:ai/openai-completions/error/retry-max-retries-default",
	"matrix:ai/openai-completions/error/retry-signal-abort-during-sleep",
	"matrix:ai/openai-completions/error/retry-signal-pre-aborted",
	"matrix:ai/openai-completions/error/retry-status-408",
	"matrix:ai/openai-completions/error/retry-status-409",
	"matrix:ai/openai-completions/error/retry-status-429",
	"matrix:ai/openai-completions/error/retry-status-5xx",
	"matrix:ai/openai-completions/error/retry-status-other",
	"matrix:ai/openai-completions/error/retry-status-undefined",
	"matrix:ai/openai-completions/header/header-retry-after",
	"matrix:ai/openai-completions/header/header-retry-after-ms",
	"matrix:ai/openai-completions/header/header-x-should-retry",
	"matrix:ai/openai-completions/option/open-aicompletions-options-max-retries",
	"matrix:ai/openai-completions/option/open-aicompletions-options-max-retry-delay-ms",
];

function parseArgs(argv) {
	const parsed = { piRoot: defaultPiRoot, out: defaultOutput, check: false };
	const rest = argv.slice(2);
	for (let i = 0; i < rest.length; i++) {
		if (rest[i] === "--check") parsed.check = true;
		else if (rest[i] === "--out") parsed.out = rest[++i];
		else if (parsed.piRoot === defaultPiRoot) parsed.piRoot = rest[i];
		else throw new Error(`unexpected argument: ${rest[i]}`);
	}
	return parsed;
}

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(
			Object.entries(value)
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

function readLock() {
	const lock = JSON.parse(readFileSync(join(repoRoot, "parity", "baseline", "upstream.lock.json"), "utf8"));
	return {
		baselineID: lock.baseline_id,
		commit: lock.source_verification.expected_commit,
		repository: lock.upstream.repository,
	};
}

function providerError(status, headers = {}, message = `provider error ${status}`) {
	return Object.assign(new Error(message), { status, headers: new Headers(headers) });
}

async function withFakeRuntime(options, run) {
	const original = {
		setTimeout: globalThis.setTimeout,
		clearTimeout: globalThis.clearTimeout,
		now: Date.now,
		random: Math.random,
	};
	const delays = [];
	let cleared = 0;
	globalThis.setTimeout = (callback, delay = 0) => {
		delays.push(Number(delay));
		if (!options.hold) queueMicrotask(callback);
		return delays.length;
	};
	globalThis.clearTimeout = () => {
		cleared++;
	};
	Date.now = () => options.now ?? fixedNow;
	Math.random = () => options.random ?? 0;
	try {
		return { observation: await run(delays), delays, cleared: () => cleared };
	} finally {
		globalThis.setTimeout = original.setTimeout;
		globalThis.clearTimeout = original.clearTimeout;
		Date.now = original.now;
		Math.random = original.random;
	}
}

async function sequence(retryProviderRequest, failures, options = {}) {
	let attempts = 0;
	try {
		const value = await retryProviderRequest(async () => {
			const failure = failures[attempts++];
			if (failure) throw failure;
			return "ok";
		}, options);
		return { attempts, outcome: { kind: "success", value } };
	} catch (error) {
		return { attempts, outcome: { kind: "error", name: error.name, message: error.message } };
	}
}

async function timedSequence(retryProviderRequest, failures, options = {}, runtime = {}) {
	const result = await withFakeRuntime(runtime, () => sequence(retryProviderRequest, failures, options));
	return canonical({ ...result.observation, delays_ms: result.delays });
}

async function capture(retryProviderRequest) {
	const retryableStatuses = {};
	for (const status of [408, 409, 429, 500, 599]) {
		retryableStatuses[status] = await timedSequence(
			retryProviderRequest,
			[providerError(status, { "retry-after-ms": "0" })],
			{ maxRetries: 1 },
		);
	}
	const rejectedStatuses = {};
	for (const status of [400, 404, 499]) {
		rejectedStatuses[status] = await timedSequence(retryProviderRequest, [providerError(status)], { maxRetries: 1 });
	}

	const delayCases = {
		retry_after_ms_precedence: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after-ms": "125", "retry-after": "9" })],
			{ maxRetries: 1 },
		),
		retry_after_seconds: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "1.5" })],
			{ maxRetries: 1 },
		),
		retry_after_date: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": new Date(fixedNow + 2_000).toUTCString() })],
			{ maxRetries: 1 },
		),
		invalid_ms_fallback: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after-ms": "NaN" })],
			{ maxRetries: 1 },
		),
		exponential_jitter: await timedSequence(
			retryProviderRequest,
			[providerError(429), providerError(429), providerError(429)],
			{ maxRetries: 3 },
			{ random: 0.4 },
		),
		fractional_jitter: await timedSequence(
			retryProviderRequest,
			[providerError(429)],
			{ maxRetries: 1 },
			{ random: 0.333 },
		),
		eight_second_cap: await timedSequence(
			retryProviderRequest,
			Array.from({ length: 6 }, () => providerError(429)),
			{ maxRetries: 6 },
			{ random: 0.8 },
		),
	};

	const capCases = {
		default: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "61" }, "rate limited")],
			{ maxRetries: 1 },
		),
		configured_equal: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "1" })],
			{ maxRetries: 1, maxRetryDelayMs: 1_000 },
		),
		configured_exceeded: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "1.001" }, "rate limited")],
			{ maxRetries: 1, maxRetryDelayMs: 1_000 },
		),
		disabled: await timedSequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "277403" })],
			{ maxRetries: 1, maxRetryDelayMs: 0 },
		),
	};

	const abortDuringSleep = await withFakeRuntime({ hold: true }, async (delays) => {
		const controller = new AbortController();
		const pending = sequence(
			retryProviderRequest,
			[providerError(429, { "retry-after": "277403" })],
			{ maxRetries: 2, maxRetryDelayMs: 0, signal: controller.signal },
		);
		for (let i = 0; delays.length === 0 && i < 10; i++) await Promise.resolve();
		if (delays.length !== 1) throw new Error("retry sleep was not scheduled");
		controller.abort();
		return pending;
	});
	const preAborted = new AbortController();
	preAborted.abort();

	return canonical({
		status: {
			retryable: retryableStatuses,
			rejected: rejectedStatuses,
			network: await timedSequence(retryProviderRequest, [providerError(undefined)], { maxRetries: 1 }),
			forced: await timedSequence(
				retryProviderRequest,
				[providerError(400, { "x-should-retry": "true" })],
				{ maxRetries: 1 },
			),
			forbidden: await timedSequence(
				retryProviderRequest,
				[providerError(429, { "x-should-retry": "false" })],
				{ maxRetries: 1 },
			),
		},
		delay: delayCases,
		cap: capCases,
		budget: {
			absent: await timedSequence(retryProviderRequest, [providerError(503)]),
			zero: await timedSequence(retryProviderRequest, [providerError(503)], { maxRetries: 0 }),
			exhausted: await timedSequence(
				retryProviderRequest,
				[providerError(503), providerError(503), providerError(503)],
				{ maxRetries: 2 },
				{ random: 0 },
			),
		},
		cancel: {
			during_sleep: { ...abortDuringSleep.observation, delays_ms: abortDuringSleep.delays, cleared_timers: abortDuringSleep.cleared() },
			pre_aborted: await timedSequence(
				retryProviderRequest,
				[providerError(429)],
				{ maxRetries: 2, signal: preAborted.signal },
			),
		},
	});
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
		input: fixture.input,
		input_sha256: fixture.input_sha256,
		actual: fixture.actual,
		hash: fixture.hash,
		exec_method: fixture.exec_method,
	});
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = readLock();
	const head = execFileSync("git", ["-C", args.piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.commit}`);
	const module = await import(pathToFileURL(join(args.piRoot, "packages", "ai", "src", "utils", "provider-retry.ts")).href);
	const input = canonical({ fixed_now: new Date(fixedNow).toISOString(), random_cases: [0, 0.333, 0.4, 0.8] });
	const actual = await capture(module.retryProviderRequest);
	const fixture = {
		schema_version: "1.0.0",
		id: "ai/openai-completions/m1-transport-retry",
		catalog_ids: catalogIDs,
		baseline_id: lock.baselineID,
		baseline_commit: lock.commit,
		deterministic: true,
		upstream: {
			module: "ai",
			repository: lock.repository,
			commit: lock.commit,
			reference: "packages/ai/src/utils/provider-retry.ts#retryProviderRequest",
		},
		input,
		input_sha256: sha256(input),
		actual,
		hash: { algorithm: "sha256", observations_sha256: sha256(actual) },
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-retry.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed retry fixture does not reproduce");
		console.error("oracle: committed retry fixture reproduces");
		return;
	}
	writeFileSync(args.out, JSON.stringify(fixture, null, 2) + "\n");
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
