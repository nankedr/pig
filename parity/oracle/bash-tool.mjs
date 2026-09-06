import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "bash-tool.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
	}
	return value;
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
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
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
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 const reference = "packages/coding-agent/src/core/tools/bash.ts";
 const {createBashTool} = await import(pathToFileURL(join(args.pi, reference)).href);
 const dir = mkdtempSync(join(tmpdir(), "pi-bash-oracle-"));
 try {
  const input = {commands:[{command:"printf 'hello\\n'"},{command:"true"},{command:"printf 'failure'; exit 7"},{command:"printf 'out'; sleep 0.05; printf 'err'"},{command:"printf 'partial'; exec sleep 5",timeout:1}]};
  const tool=createBashTool(dir);
  const results=[];
  for(const command of input.commands){
   try {const result=await tool.execute("bash",command);results.push({text:result.content[0].text,isError:false});}
   catch(error){results.push({text:error.message,isError:true});}
  }
  const observation={outcome:{results},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/bash-tool",catalog_id:"contract:codingagent/bash-tool",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/bash-tool.mjs <locked-pi-checkout>",platform:"darwin-arm64",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});
