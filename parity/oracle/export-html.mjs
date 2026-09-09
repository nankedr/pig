import { createHash } from "node:crypto";
import { spawn, execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { once } from "node:events";
const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
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
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value).replaceAll("\u2028", "\\u2028").replaceAll("\u2029", "\\u2029");
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


const pi = process.argv[2];
const lock = JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json")));
if (execFileSync("git",["-C",pi,"rev-parse","HEAD"],{encoding:"utf8"}).trim() !== lock.upstream.commit) throw Error("baseline mismatch");
if (execFileSync("git",["-C",pi,"status","--porcelain=v1","--untracked-files=no"],{encoding:"utf8"}).trim()) throw Error("dirty Pi sources");
const {exportFromFile} = await import(pathToFileURL(join(pi,"packages/coding-agent/src/core/export-html/index.ts")));
const dir=mkdtempSync(join(tmpdir(),"pi-html-"));
try {
 const input=readFileSync(join(root,"parity/export-html/session.jsonl"),"utf8");
 const source=join(dir,"session.jsonl"), htmlPath=join(dir,"pi.html");
 writeFileSync(source,input);
 await exportFromFile(source,htmlPath);
 const html=readFileSync(htmlPath,"utf8");
 const data=JSON.parse(Buffer.from(html.match(/id="session-data" type="application\/json">([^<]+)/)[1],"base64"));
 const {observeExport}=await import("../export-html/browser.mjs");
 const rendered=await observeExport(htmlPath);
 const c={schema_version:"1.0.0",id:"go-sdk/codingagent/html-export",catalog_id:"contract:codingagent/html-export",surface:"go-sdk",input:{session:input},observe:["outcome","side_effects"]};
 const observation={outcome:{data,rendered},side_effects:[]};
 const reference="packages/coding-agent/src/core/export-html/index.ts#exportFromFile";
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/export-html.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 const output=join(root,"parity/oracle/fixtures/export-html.json");
 if(process.argv.includes("--check")){const old=JSON.parse(readFileSync(output));fixture.environment.node=old.environment.node;if(JSON.stringify(old)!==JSON.stringify(fixture))throw Error("fixture drift");console.log("verified export HTML source and browser fixture");}
 else {writeFileSync(output,JSON.stringify(fixture,null,2)+"\n");console.log("wrote export HTML source and browser fixture");}
} finally {rmSync(dir,{recursive:true,force:true});}
