import { caseDigest, observationDigest } from "./fixture-hash.mjs";
import { execFileSync } from "node:child_process";
import { realpathSync, chmodSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "prompt-templates.json");

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
 const {expandPromptTemplate} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/prompt-templates.ts")));
 const scenarios = [
  {name:"missing",files:{},calls:["/missing hi"]},
  {name:"metadata",files:{"agent/prompts/review.md":"---\ndescription: Review code\nargument-hint: <file> [focus]\n---\nReview $1; ${2:-correctness}; ${@:2}; $ARGUMENTS", "agent/prompts/bad.md":"---\ndescription: [broken\n---\nBAD", "agent/prompts/fallback.md":"\n  First line\nsecond", "agent/prompts/long.md":"x".repeat(65), "agent/prompts/crlf.md":"---\r\ndescription: >-\r\n  folded\r\n  description\r\n---\r\n BODY \r\n", "agent/prompts/empty.md":"---\n---\n", "agent/prompts/null.md":"---\nnull\n---\nNULL", "agent/prompts/duplicate.md":"---\ndescription: one\ndescription: two\n---\nBAD", "agent/prompts/.hidden.md":"HIDDEN", "agent/prompts/nested/child.md":"NESTED"},calls:["/review 'a b' security tests", "/review", "/review \"$1\" \"$@\"", "/unknown x", "plain", "/review\nfoo"]},
  {name:"collisions",trusted:true,files:{"agent/prompts/same.md":"GLOBAL", "repo/.pi/prompts/same.md":"PROJECT", "extra/same.md":"EXPLICIT", "repo/.pi/prompts/project.md":"PROJECT_ONLY"},paths:["../extra", "../missing"],calls:["/same", "/project"]},
  {name:"untrusted",files:{"agent/prompts/same.md":"GLOBAL", "repo/.pi/prompts/same.md":"SECRET"},calls:["/same"]},
  {name:"disabled",disabled:true,trusted:true,files:{"agent/prompts/same.md":"GLOBAL", "repo/.pi/prompts/same.md":"PROJECT", "extra/same.md":"EXPLICIT"},paths:["../extra"],calls:["/same"]},
  {name:"disabled-empty",disabled:true,files:{"agent/prompts/same.md":"GLOBAL"},calls:["/same"]},
  {name:"explicit-project",files:{"repo/.pi/prompts/same.md":"EXPLICIT"},paths:[".pi/prompts/same.md"],calls:["/same"]},
  {name:"settings",trusted:true,global:["more", "!skip.md"],project:["more"],files:{"agent/more/same.md":"GLOBAL_SETTINGS", "repo/.pi/more/same.md":"PROJECT_SETTINGS", "agent/prompts/same.md":"GLOBAL", "repo/.pi/prompts/same.md":"PROJECT", "agent/prompts/skip.md":"SKIP"},calls:["/same"]},
  {name:"ignore",files:{"agent/prompts/.gitignore":"skip.md\n", "agent/prompts/skip.md":"SKIP", "agent/prompts/keep.md":"KEEP"},calls:["/keep"]},
  {name:"arguments",files:{"agent/prompts/args.md":"$1|$2|$0|$9|${1:-$@}|${9:-$1}|${@:-all}|${ARGUMENTS:-all}|${@:0:2}|${@:2:1}|${@:9}|${@:1:0}"},calls:["/args a 'b c' d", "/args", "/args '' \"x y\"", "/args 'unclosed", "/args $1 $@", "/args a", "/args a"]},
  {name:"yaml-shapes",files:{"agent/prompts/scalar.md":"---\nhello\n---\nSCALAR", "agent/prompts/list.md":"---\n- hello\n---\nLIST", "agent/prompts/date.md":"---\ndescription: 2026-09-14\n---\nDATE", "agent/prompts/merge.md":"---\nbase: &base {description: inherited}\n<<: *base\n---\nMERGE", "agent/prompts/alias.md":"---\nx: &text 'Aliased description'\ndescription: *text\n---\nALIAS"},calls:["/date", "/merge"]},
  {name:"settings-recursive",global:["more", "!**/skip*.md", "+more/skip-keep.md", "-more/skip-no.md"],files:{"agent/more/a.md":"A","agent/more/deep/b.md":"B","agent/more/deep/skip.md":"SKIP","agent/more/skip-keep.md":"KEEP","agent/more/skip-no.md":"NO", "agent/more/.hidden.md":"HIDDEN", "agent/more/.ignore":"ignored.md\n", "agent/more/ignored.md":"IGNORED"},calls:["/b", "/skip-keep"]},
  {name:"explicit-url",paths:["file://$ROOT/extra/review.md"],files:{"extra/review.md":"URL $1"},calls:["/review file"]},
  {name:"duplicate-explicit-directory",paths:["../agent/prompts", "../agent/prompts"],files:{"agent/prompts/review.md":"SAME"},calls:["/review"]},
 ];
 const outcomes=[];
 for (const scenario of scenarios) {
  const dir=realpathSync(mkdtempSync(join(tmpdir(),"pi-prompt-templates-")));
  try {
   for(const [path,content] of Object.entries(scenario.files)){mkdirSync(dirname(join(dir,path)),{recursive:true});writeFileSync(join(dir,path),content);}
   const cwd=join(dir,"repo"),agentDir=join(dir,"agent");mkdirSync(cwd,{recursive:true});mkdirSync(agentDir,{recursive:true});
   writeFileSync(join(agentDir,"settings.json"),JSON.stringify({prompts:scenario.global??[]}));
   mkdirSync(join(cwd,".pi"),{recursive:true});writeFileSync(join(cwd,".pi/settings.json"),JSON.stringify({prompts:scenario.project??[]}));
   const settings=SettingsManager.create(cwd,agentDir);settings.setProjectTrusted(scenario.trusted??false);
   const loader=new DefaultResourceLoader({cwd,agentDir,settingsManager:settings,noExtensions:true,noSkills:true,noPromptTemplates:scenario.disabled,noThemes:true,noContextFiles:true,additionalPromptTemplatePaths:scenario.paths?.map(p=>p.replaceAll("$ROOT",dir))??[]});
   await loader.reload();
   const {prompts,diagnostics}=loader.getPrompts();
   const normalize=v=>JSON.parse(JSON.stringify(v).replaceAll(dir,"$ROOT"));
   outcomes.push({name:scenario.name,prompts:normalize(prompts),diagnostics:normalize(diagnostics),expanded:scenario.calls.map((text,i)=>scenario.name==="arguments"&&i===6?text:expandPromptTemplate(text,prompts))});
  }finally{rmSync(dir,{recursive:true,force:true});}
 }
 const c={schema_version:"1.0.0",id:"sdk/codingagent/prompt-templates",catalog_id:"contract:codingagent/prompt-templates",surface:"go-sdk",input:{scenarios},observe:["outcome","side_effects"]};
 const observation={outcome:outcomes,side_effects:[]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/prompt-templates.mjs <locked-pi-checkout>",platform:"posix",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
}
main().catch(e=>{console.error(e);process.exit(1)});
