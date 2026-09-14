import { caseDigest, observationDigest } from "./fixture-hash.mjs";
import { execFileSync } from "node:child_process";
import { realpathSync, chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "system-prompts.json");

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
  {name:"default",files:{}},
  {name:"global",files:{"agent/SYSTEM.md":"GLOBAL","agent/APPEND_SYSTEM.md":"GLOBAL_APPEND"}},
  {name:"trusted-project",trusted:true,files:{"agent/SYSTEM.md":"GLOBAL","agent/APPEND_SYSTEM.md":"GLOBAL_APPEND","repo/.pi/SYSTEM.md":"PROJECT","repo/.pi/APPEND_SYSTEM.md":"PROJECT_APPEND","repo/AGENTS.md":"CONTEXT"}},
  {name:"untrusted-project",files:{"agent/SYSTEM.md":"GLOBAL","agent/APPEND_SYSTEM.md":"GLOBAL_APPEND","repo/.pi/SYSTEM.md":"SECRET","repo/.pi/APPEND_SYSTEM.md":"SECRET_APPEND"}},
  {name:"explicit-values",system:"EXPLICIT",append:["FIRST","","SECOND"],files:{"agent/SYSTEM.md":"GLOBAL","agent/APPEND_SYSTEM.md":"GLOBAL_APPEND"}},
  {name:"explicit-files",system:"$ROOT/repo/.pi/SYSTEM.md",append:["$ROOT/repo/.pi/APPEND_SYSTEM.md","TAIL"],files:{"repo/.pi/SYSTEM.md":"EXPLICIT_FILE","repo/.pi/APPEND_SYSTEM.md":"EXPLICIT_APPEND"}},
  {name:"explicit-empty",system:"",append:[],files:{"agent/SYSTEM.md":"IGNORED","agent/APPEND_SYSTEM.md":"IGNORED_APPEND"}},
  {name:"empty-files",trusted:true,files:{"repo/.pi/SYSTEM.md":"","repo/.pi/APPEND_SYSTEM.md":"","agent/SYSTEM.md":"IGNORED","agent/APPEND_SYSTEM.md":"IGNORED_APPEND"}},
  {name:"missing-path-is-value",system:"./missing-system.md",append:["./missing-append.md"],files:{}},
  {name:"directory-failure",system:"$ROOT/directory",append:["$ROOT/directory"],files:{"directory/child":"x"}},
  {name:"unreadable",system:"$ROOT/private.md",append:["$ROOT/private.md"],unreadable:"private.md",files:{"private.md":"SECRET"}},
  {name:"disabled-resources",disabled:true,files:{"agent/SYSTEM.md":"GLOBAL","agent/APPEND_SYSTEM.md":"GLOBAL_APPEND","repo/AGENTS.md":"IGNORED_CONTEXT"}},
  {name:"relative-path",system:"./relative.md",append:["./relative.md"],files:{"repo/relative.md":"RELATIVE"}},
  {name:"file-url-is-value",system:"file:///missing.md",append:["~/missing.md"],files:{}},
 ];
 const outcomes=[];
 for (const scenario of scenarios) {
  const dir=realpathSync(mkdtempSync(join(tmpdir(),"pi-system-prompts-")));
  const originalError=console.error, originalCwd=process.cwd();
  try {
   for(const [path,content] of Object.entries(scenario.files)){mkdirSync(dirname(join(dir,path)),{recursive:true});writeFileSync(join(dir,path),content);}
   const cwd=join(dir,"repo"),agentDir=join(dir,"agent");mkdirSync(cwd,{recursive:true});mkdirSync(agentDir,{recursive:true});process.chdir(cwd);
   if(scenario.unreadable)chmodSync(join(dir,scenario.unreadable),0);
   const expand=s=>s.replaceAll("$ROOT",dir);
   const warnings=[];console.error=(...args)=>warnings.push(args.join(" "));
   const settings=SettingsManager.inMemory();settings.setProjectTrusted(scenario.trusted??false);
   const loader=new DefaultResourceLoader({cwd,agentDir,settingsManager:settings,noExtensions:true,noSkills:true,noPromptTemplates:true,noThemes:true,noContextFiles:scenario.disabled,systemPrompt:scenario.system===undefined?undefined:expand(scenario.system),appendSystemPrompt:scenario.append?.map(expand)});
   await loader.reload();
   const system=loader.getSystemPrompt(),append=loader.getAppendSystemPrompt();
   const prompt=buildSystemPrompt({cwd,customPrompt:system,appendSystemPrompt:append.join("\n\n"),contextFiles:loader.getAgentsFiles().agentsFiles,selectedTools:[]});
   const normalize=s=>s.replaceAll(dir,"$ROOT");
   outcomes.push({name:scenario.name,system:system===undefined?null:normalize(system),append:append.map(normalize),source:loader.getSystemPromptSource()?.path?normalize(loader.getSystemPromptSource().path):null,appendSources:loader.getAppendSystemPromptSources().map(s=>normalize(s.path)),warning:warnings.some(w=>w.includes("Could not read")),custom:!!system,context:prompt.includes("CONTEXT"),appended:append.filter(Boolean).every(s=>prompt.includes(s))});
  }finally{process.chdir(originalCwd);console.error=originalError;if(scenario.unreadable)chmodSync(join(dir,scenario.unreadable),0o600);rmSync(dir,{recursive:true,force:true});}
 }
 const c={schema_version:"1.0.0",id:"sdk/codingagent/system-prompts",catalog_id:"contract:codingagent/system-prompts",surface:"go-sdk",input:{scenarios},observe:["outcome","side_effects"]};
 const observation={outcome:outcomes,side_effects:[]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/system-prompts.mjs <locked-pi-checkout>",platform:"posix non-root",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
}
main().catch(e=>{console.error(e);process.exit(1)});
