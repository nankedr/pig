import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "credentials.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
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


function caseDigest(value) {
	const projected = {
		schema_version: value.schema_version,
		id: value.id,
		catalog_id: value.catalog_id,
		surface: value.surface,
		input: canonical(value.input),
		observe: value.observe,
	};
	return `sha256:${createHash("sha256").update(canonicalJSONString(JSON.parse(JSON.stringify(projected)))).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(canonicalJSONString(JSON.parse(JSON.stringify(projected)))).digest("hex")}`;
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


async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("baseline mismatch");
 const reference = "packages/coding-agent/src/core/auth-storage.ts";
 const {AuthStorage, readStoredCredential} = await import(pathToFileURL(join(args.pi, reference)));
 const dir = mkdtempSync(join(tmpdir(), "pi-credentials-"));
 try {
  const path = join(dir,"auth.json"), store = AuthStorage.create(path);
  const values = ["literal-key", "$PIG_PARITY_LEFT", "${PIG_PARITY_LEFT}_$PIG_PARITY_RIGHT", "$$PIG_PARITY_LEFT", "$!literal-$PIG_PARITY_RIGHT", "${bad-name}", "!printf '  command-key  '"];
  const keys = [];
  for (const key of values) {
   await store.modify("deepseek", async()=>({type:"api_key",key,env:{PIG_PARITY_LEFT:"left",PIG_PARITY_RIGHT:"right"},custom:{preserved:17}}));
   keys.push((await store.read("deepseek")).key);
  }
  const raw = readStoredCredential("deepseek", path);
  const unchanged = await store.modify("deepseek", async()=>undefined);
  await store.modify("openai",async()=>({type:"api_key",key:"other"}));
  const listed = (await store.list()).map(x=>x.providerId).sort();
  await store.delete("deepseek");
  const {statSync} = await import("node:fs");
  const observation = {outcome:{keys,rawKey:raw.key,unchanged:unchanged.key===raw.key,extra:raw.custom,listed,deleted:(await store.read("deepseek"))===undefined,other:(await store.read("openai")).key,fileMode:statSync(path).mode&0o777},side_effects:[]};
  const c = {schema_version:"1.0.0",id:"sdk/codingagent/credentials",catalog_id:"contract:config/auth-json",surface:"go-sdk",input:{values,env:{PIG_PARITY_LEFT:"left",PIG_PARITY_RIGHT:"right"}},observe:["outcome","side_effects"]};
  const fixture = {schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/credentials.mjs <locked-pi-checkout>",platform:"darwin/linux",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(e=>{console.error(e);process.exit(1)});
