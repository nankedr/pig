import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { chmodSync, symlinkSync, existsSync, mkdirSync, readdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "context-files.json");

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
 const reference = "packages/coding-agent/src/core/resource-loader.ts";
 const {DefaultResourceLoader} = await import(pathToFileURL(join(args.pi, reference)));
 const {SettingsManager} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/settings-manager.ts")));
 const {buildSystemPrompt} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/system-prompt.ts")));
 const scenarios = [
  {name:"layering", files:{"agent/AGENTS.override.md":"GLOBAL", "agent/AGENTS.md":"IGNORED", "AGENTS.md":"ABOVE_REPO", "repo/.git/HEAD":"ref: refs/heads/main", "repo/AGENTS.md":"REPO", "repo/child/CLAUDE.md":"LEAF"}},
  {name:"empty-wins", files:{"repo/child/AGENTS.override.md":"", "repo/child/AGENTS.md":"IGNORED"}},
  {name:"missing", files:{}},
  {name:"uppercase", files:{"repo/child/CLAUDE.MD":"UPPER"}},
  {name:"file-link", files:{"target.md":"LINKED"}, links:{"repo/child/AGENTS.md":"../../target.md"}},
  {name:"broken-link", files:{"repo/child/CLAUDE.md":"FALLBACK"}, links:{"repo/child/AGENTS.md":"missing"}},
  {name:"directory-candidate", files:{"repo/child/AGENTS.md/ignored":"DIRECTORY", "repo/child/CLAUDE.md":"FALLBACK"}},
  {name:"unreadable", files:{"repo/child/AGENTS.override.md":"UNREADABLE", "repo/child/AGENTS.md":"FALLBACK"}, unreadable:"repo/child/AGENTS.override.md"},
  {name:"disabled", files:{"agent/AGENTS.md":"GLOBAL", "repo/child/AGENTS.md":"LEAF"}, disabled:true},
  {name:"global-dedup", files:{"repo/AGENTS.md":"ONCE"}, agentDir:"repo"},
  {name:"cwd-link", files:{"repo/AGENTS.md":"LEXICAL_PARENT", "target/AGENTS.md":"TARGET"}, links:{"repo/child":"../target"}},
  {name:"file-url", files:{"repo/child/AGENTS.md":"URL"}, fileURL:true},
  {name:"nested-worktree", files:{"repo/AGENTS.md":"SHADOWED", "repo/.git/worktrees/feature/HEAD":"ref: refs/heads/feature", "repo/.git/worktrees/feature/commondir":"../..", "repo/child/.git":"gitdir: ../.git/worktrees/feature", "repo/child/AGENTS.md":"WORKTREE"}},
  {name:"incomplete-worktree", files:{"repo/AGENTS.md":"SHADOWED", "repo/.git/worktrees/feature/commondir":"../..", "repo/child/.git":"gitdir: ../.git/worktrees/feature", "repo/child/AGENTS.md":"WORKTREE"}},
 ];
 const outcomes=[];
 for (const scenario of scenarios) {
  const dir=mkdtempSync(join(tmpdir(),"pi-context-files-"));
  const originalError=console.error;
  try {
   for(const [path,content] of Object.entries(scenario.files)){mkdirSync(dirname(join(dir,path)),{recursive:true});writeFileSync(join(dir,path),content);}
   for(const [path,target] of Object.entries(scenario.links??{})){mkdirSync(dirname(join(dir,path)),{recursive:true});symlinkSync(target,join(dir,path));}
   const cwd=join(dir,"repo/child"), agentDir=join(dir,scenario.agentDir??"agent");
   mkdirSync(cwd,{recursive:true});mkdirSync(agentDir,{recursive:true});
   if(scenario.unreadable)chmodSync(join(dir,scenario.unreadable),0);
   const warnings=[];console.error=(...args)=>warnings.push(args.join(" "));
   const loader=new DefaultResourceLoader({cwd:scenario.fileURL?pathToFileURL(cwd).href:cwd,agentDir,settingsManager:SettingsManager.inMemory(),noExtensions:true,noSkills:true,noPromptTemplates:true,noThemes:true,noContextFiles:scenario.disabled});
   await loader.reload();
   const files=loader.getAgentsFiles().agentsFiles.filter(f=>f.path.startsWith(dir+"/"));
   const prompt=buildSystemPrompt({cwd,contextFiles:files,selectedTools:[]});
   outcomes.push({name:scenario.name,files:files.map(f=>({path:scenario.name==="uppercase"?f.path.slice(dir.length+1).toLowerCase():f.path.slice(dir.length+1),content:f.content})),inPrompt:files.every(f=>prompt.includes(`<project_instructions path="${f.path}">\n${f.content}\n</project_instructions>`)),warning:warnings.some(w=>w.includes("Could not read"))});
  }finally{console.error=originalError;if(scenario.unreadable)chmodSync(join(dir,scenario.unreadable),0o600);rmSync(dir,{recursive:true,force:true});}
 }
 const c={schema_version:"1.0.0",id:"sdk/codingagent/context-files",catalog_id:"contract:codingagent/context-files",surface:"go-sdk",input:{scenarios},observe:["outcome","side_effects"]};
 const observation={outcome:outcomes,side_effects:[]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/context-files.mjs <locked-pi-checkout>",platform:"posix non-root",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
}
main().catch(e=>{console.error(e);process.exit(1)});
