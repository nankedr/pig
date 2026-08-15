// run.mjs — the Pi Oracle harness for Milestone 0 (issue #22).
//
// Node is used ONLY here and by the surface extractor (#21); normal Pig
// build/test never runs this and stays pure-Go with no Node. The Oracle is the
// on-demand bridge to the frozen Pi baseline: it accepts only a checkout whose
// HEAD matches the lock, drives one deterministic protocol case through Pi's
// real CBOR encoder and frame codec, and records the raw output plus full
// provenance to a committed fixture. The Go side (internal/oracle) then replays
// that fixture fully offline and pure-stdlib, independently re-deriving the
// frame header, so parity is a genuine cross-implementation check rather than a
// transliteration of Pi's bit-shifts.
//
// The captured case is the canonical ClientHello {type:"hello",version:1} — the
// mandatory first client protocol message. Pi's codec builds a client frame as
// encodeFrame(encodeCbor(message)) (packages/protocol/src/codec.ts); both
// encodeCbor and encodeFrame are zero-dependency, so the Oracle needs no npm
// install. The wire bytes are a pure function of (locked source, input): the
// same frame and hash reproduce on any host, which is what deterministic:true
// asserts and what --check re-verifies.
//
// Usage:
//   node run.mjs [<pi-checkout-root>] [--out <path>]   capture the fixture
//   node run.mjs --check [<pi-checkout-root>]           verify it reproduces
// Default checkout: <repo>/.upstream/pi. Default out: ./fixtures/protocol-frame.json.

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const SCHEMA_VERSION = "1.0.0";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");

function parseArgs(argv) {
	const args = {
		piRoot: null,
		out: join(here, "fixtures", "protocol-frame.json"),
		check: false,
	};
	const rest = argv.slice(2);
	for (let i = 0; i < rest.length; i++) {
		if (rest[i] === "--check") args.check = true;
		else if (rest[i] === "--out") args.out = rest[++i];
		else if (!args.piRoot) args.piRoot = rest[i];
	}
	if (!args.piRoot) args.piRoot = join(repoRoot, ".upstream", "pi");
	return args;
}

// readLock returns the locked upstream commit and baseline id. The Oracle
// refuses to run against any checkout whose HEAD differs from this commit.
function readLock() {
	const lockPath = join(repoRoot, "parity", "baseline", "upstream.lock.json");
	const lock = JSON.parse(readFileSync(lockPath, "utf8"));
	const commit = lock?.source_verification?.expected_commit ?? lock?.upstream?.commit;
	const baselineID = lock?.baseline_id;
	if (!commit) throw new Error(`lock ${lockPath} has no expected commit`);
	if (!baselineID) throw new Error(`lock ${lockPath} has no baseline_id`);
	return { commit, baselineID, repository: lock.upstream.repository };
}

// assertCheckoutAtLock verifies the Pi checkout HEAD equals the locked commit.
function assertCheckoutAtLock(piRoot, expectedCommit) {
	let head;
	try {
		head = execFileSync("git", ["-C", piRoot, "rev-parse", "HEAD"], {
			encoding: "utf8",
		}).trim();
	} catch (error) {
		throw new Error(`unable to resolve HEAD at ${piRoot}: ${error.message}`);
	}
	if (head !== expectedCommit) {
		throw new Error(
			`checkout HEAD ${head} != locked commit ${expectedCommit}; the Oracle only runs against the frozen baseline`,
		);
	}
}

// encodeCase drives the canonical ClientHello through Pi's real codec, returning
// the intermediate CBOR bytes and the framed output. The message key order is
// pinned because Pi's CBOR encoder preserves object insertion order.
async function encodeCase(piRoot) {
	const src = join(piRoot, "packages", "protocol", "src");
	const { encodeCbor } = await import(pathToFileURL(join(src, "cbor", "index.ts")).href);
	const { encodeFrame, DEFAULT_MAX_FRAME_LENGTH } = await import(
		pathToFileURL(join(src, "framing.ts")).href
	);

	// The mandatory first client message. type precedes version to match the
	// ClientHelloSchema field order in packages/protocol/src/schemas.ts.
	const message = { type: "hello", version: 1 };

	// This is exactly how packages/protocol/src/codec.ts frames a validated
	// client message: encodeFrame(encodeCbor(message, { maxByteLength })).
	const cbor = encodeCbor(message, { maxByteLength: DEFAULT_MAX_FRAME_LENGTH });
	const frame = encodeFrame(cbor);
	return { message, cbor, frame };
}

const hex = (bytes) => Buffer.from(bytes).toString("hex");

// buildFixture assembles the deterministic, provenance-carrying fixture object.
// The field order here is the on-disk order; it is stable across captures on the
// same host so re-running produces a minimal (env-only) diff.
function buildFixture(lock, { message, cbor, frame }) {
	return {
		schema_version: SCHEMA_VERSION,
		id: "protocol/frame",
		catalog_id: "contract:protocol/frame",
		baseline_id: lock.baselineID,
		baseline_commit: lock.commit,
		deterministic: true,
		upstream: {
			module: "protocol",
			repository: lock.repository,
			commit: lock.commit,
			reference: "packages/protocol/src/framing.ts#encodeFrame",
		},
		input: {
			description:
				"Canonical ClientHello: the mandatory first client protocol message (ClientHelloSchema, PROTOCOL_VERSION=1).",
			message,
			encoding: "encodeFrame(encodeCbor(message, { maxByteLength }))",
		},
		raw_output: {
			encoding: "hex",
			cbor_hex: hex(cbor),
			cbor_length: cbor.byteLength,
			frame_hex: hex(frame),
			frame_length: frame.byteLength,
			header_hex: hex(frame.subarray(0, 4)),
			header_length: 4,
		},
		hash: {
			algorithm: "sha256",
			frame_sha256: createHash("sha256").update(frame).digest("hex"),
		},
		env: {
			node_version: process.version,
			platform: process.platform,
			arch: process.arch,
		},
		exec_method: "node-native-typescript-import",
	};
}

// protocolFacts extracts the environment-independent wire facts that must
// reproduce byte-for-byte: the raw output and its hash. --check compares only
// these, so host provenance (env) never causes a false reproduction failure.
function protocolFacts(fixture) {
	return JSON.stringify({ raw_output: fixture.raw_output, hash: fixture.hash });
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = readLock();
	assertCheckoutAtLock(args.piRoot, lock.commit);

	const encoded = await encodeCase(args.piRoot);
	const fixture = buildFixture(lock, encoded);

	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (protocolFacts(committed) !== protocolFacts(fixture)) {
			console.error("oracle: committed fixture does NOT reproduce the protocol facts");
			console.error("  committed:", protocolFacts(committed));
			console.error("  recomputed:", protocolFacts(fixture));
			process.exit(1);
		}
		console.error(
			`oracle: committed fixture reproduces (frame ${fixture.raw_output.frame_length} bytes, sha256 ${fixture.hash.frame_sha256})`,
		);
		return;
	}

	mkdirSync(dirname(args.out), { recursive: true });
	writeFileSync(args.out, JSON.stringify(fixture, null, 2) + "\n");
	console.error(
		`oracle: captured ${fixture.id} -> ${args.out} (frame ${fixture.raw_output.frame_length} bytes, sha256 ${fixture.hash.frame_sha256})`,
	);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
