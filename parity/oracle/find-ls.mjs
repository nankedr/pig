import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "find-ls.json");

function canonical(value) {
 if (typeof value === "number" && value !== 0) {
  const [mantissa, power = "0"] = String(value).split("e"), [integer, fraction = ""] = mantissa.split(".");
  const digits = integer + fraction, trimmed = digits.replace(/0+$/, ""), exponent = Number(power) - fraction.length + digits.length - trimmed.length;
  return JSON.rawJSON(exponent === 0 ? trimmed : `${trimmed}e${exponent}`);
 }
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, child]) => [key, canonical(child)]));
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
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], {encoding:"utf8"}).trim()) throw new Error("Pi checkout has tracked changes");
 const {getToolPath} = await import(pathToFileURL(join(args.pi,"packages/coding-agent/src/utils/tools-manager.ts")).href);
 if (!getToolPath("fd")) throw new Error("prepare fd before running the Oracle; downloads are not allowed");
 const reference = "packages/coding-agent/src/core/tools/find.ts";
 const {createFindTool, createFindToolDefinition} = await import(pathToFileURL(join(args.pi, reference)).href);
 const {createLsTool, createLsToolDefinition} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/tools/ls.ts")).href);
 const {mkdirSync, symlinkSync} = await import("node:fs");
 const dir = mkdtempSync(join(tmpdir(), "pi-find-ls-"));
 try {
  const input = {
   files: {".hidden":"hidden", "Alpha.txt":"alpha", "beta.txt":"beta", "é.txt":"accent", "z.txt":"z", "space name.txt":"space", "a/.gitignore":"ignored.txt\n", "a/ignored.txt":"ignored", "a/kept.txt":"kept", "b/ignored.txt":"sibling", "src/deep/test.spec.ts":"spec", ".secret/hidden.txt":"hidden"},
   cases: [
    {tool:"ls",args:{}}, {tool:"ls",args:{limit:2}}, {tool:"ls",args:{limit:2.5}}, {tool:"ls",args:{limit:0}}, {tool:"ls",args:{path:"empty"}},
    {tool:"ls",args:{path:"missing"}}, {tool:"ls",args:{path:"Alpha.txt"}}, {tool:"ls",args:{path:"a",limit:3}},
    {tool:"find",args:{pattern:"*.txt"},sort:true}, {tool:"find",args:{pattern:"src/**/*.spec.ts"}},
    {tool:"find",args:{pattern:"--help"}}, {tool:"find",args:{pattern:"*.txt",path:"a"}},
    {tool:"find",args:{pattern:"*.txt",path:"a",limit:1}},
    {tool:"find",args:{pattern:"**",path:"/"},custom:["/home/user/project/","/home/user/project/file.txt"]},
    {tool:"find",args:{pattern:"**",limit:2},custom:["{cwd}/beta.txt","{cwd}/Alpha.txt"]},
    {tool:"find",args:{pattern:"**",limit:1},custom:["a/","b/","file\\"]},
    {tool:"find",args:{pattern:"**"},custom:[]},
    {tool:"find",args:{pattern:"**"},custom:["x".repeat(52000)]}
   ]
  };
  for (const [name,content] of Object.entries(input.files)) {mkdirSync(dirname(join(dir,name)),{recursive:true});writeFileSync(join(dir,name),content);}
  mkdirSync(join(dir,"empty")); symlinkSync("a",join(dir,"link")); symlinkSync("missing",join(dir,"broken"));
  const results=[];
  for (const item of input.cases) {
   const options=item.custom ? {operations:{exists:()=>true,glob:()=>item.custom.map(p=>p.replaceAll("{cwd}",dir))}} : undefined;
   const tool=item.tool==="ls"?createLsTool(dir):createFindTool(dir,options);
   try {const result=await tool.execute("locate",item.args);let text=result.content[0].text;if(item.sort)text=text.split("\n").sort().join("\n");results.push({text,details:result.details??null,isError:false});}
   catch(e){results.push({text:e.message.replaceAll(dir,"{cwd}"),details:null,isError:true});}
  }
  const metadata=[createFindToolDefinition(dir),createLsToolDefinition(dir)].map(d=>({name:d.name,label:d.label,description:d.description,parameters:d.parameters,promptSnippet:d.promptSnippet}));
  const observation={outcome:{results,metadata},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/find-ls",catalog_id:"contract:codingagent/find-ls",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/find-ls.mjs <locked-pi-checkout>",platform:"darwin-arm64",environment:{node:process.version,fd:execFileSync(getToolPath("fd"),["--version"],{encoding:"utf8"}).trim(),oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});
