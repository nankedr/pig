import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "write-tool.json");

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
 const reference = "packages/coding-agent/src/core/tools/write.ts";
 const {createWriteTool, createWriteToolDefinition} = await import(pathToFileURL(join(args.pi, reference)).href);
 const {createReadTool} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/tools/read.ts")).href);
 const dir = mkdtempSync(join(tmpdir(), "pi-write-tool-"));
 try {
  const input = {cwdForms:["tilde","file-url"],writes:[
   {path:"nested/dir/result.txt",content:"Hello\n"},
   {path:"nested/dir/result.txt",content:"你好🙂\r\n"},
   {path:"@space\u202fname.txt",content:"space"},
   {path:"empty.txt",content:""},
   {path:"../parent.txt",content:"parent"}
  ]};
  const cwd=join(dir,"workspace");
  const write=createWriteTool(cwd), read=createReadTool(cwd);
  const results=[];
  for (const args of input.writes) {
   const result=await write.execute("write",args);
   const back=await read.execute("read",{path:args.path});
   results.push({text:result.content[0].text,detailsEmpty:result.details==null,read:back.content[0].text});
  }
  const cwdResults=[];
  const savedHome=process.env.HOME, savedProfile=process.env.USERPROFILE;
  process.env.HOME=dir; process.env.USERPROFILE=dir;
  try {
   for(const form of input.cwdForms){
    const tool=createWriteTool(form==="tilde"?"~/workspace":pathToFileURL(cwd).href);
    const result=await tool.execute("cwd",{path:"cwd.txt",content:form});
    const back=await read.execute("read",{path:"cwd.txt"});
    cwdResults.push({text:result.content[0].text,read:back.content[0].text});
   }
  } finally {if(savedHome===undefined)delete process.env.HOME;else process.env.HOME=savedHome;if(savedProfile===undefined)delete process.env.USERPROFILE;else process.env.USERPROFILE=savedProfile;}
  const definition=createWriteToolDefinition(cwd);
  const metadata={name:definition.name,label:definition.label,description:definition.description,parameters:definition.parameters,promptSnippet:definition.promptSnippet,promptGuidelines:definition.promptGuidelines};
  const observation={outcome:{results,metadata,cwdResults},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/write-tool",catalog_id:"contract:codingagent/write-tool",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/write-tool.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});
