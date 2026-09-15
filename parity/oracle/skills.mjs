import { caseDigest, observationDigest } from "./fixture-hash.mjs";
import { execFileSync } from "node:child_process";
import { realpathSync, symlinkSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "skills.json");

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
 const {AgentSession} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/agent-session.ts")));
 const {buildSystemPrompt} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/system-prompt.ts")));
 const skill=(name,extra="",body="Run scripts/check.sh using the supplied task.")=>`---\nname: ${name}\ndescription: Use ${name} for this task\n${extra}---\n${body}`;
 const scenarios = [
  {name:"missing",files:{},paths:["../absent","../plain.txt"],calls:["/skill:missing hi"],extraFiles:{"plain.txt":"TEXT"}},
  {name:"metadata",files:{"agent/skills/review/SKILL.md":skill("review"),"agent/skills/manual/SKILL.md":skill("manual","disable-model-invocation: true\n"),"agent/skills/fallback/SKILL.md":"---\ndescription: Fallback name\n---\nFALLBACK", "agent/skills/bad/SKILL.md":skill("-Bad--"),"agent/skills/empty/SKILL.md":"---\nname: empty\n---\nSKIP", "agent/skills/space/SKILL.md":"---\ndescription: '   '\n---\nSKIP", "agent/skills/long/SKILL.md":skill("x".repeat(65)).replace("Use "+"x".repeat(65)+" for this task","d".repeat(1025)), "agent/skills/hidden/SKILL.md":skill("hidden","disable-model-invocation: 'true'\n")},calls:["/skill:review task", "/skill:manual explicit", "/skill:fallback", "/skill:unknown hi", "/skill:review\nhi", "/skill:review   a  b  "]},
  {name:"discovery",trusted:true,files:{"agent/skills/root.md":skill("root"),"agent/skills/nested/ignore.md":skill("skip-nested"),"agent/skills/nested/deep/SKILL.md":skill("deep"),"agent/skills/stop/SKILL.md":skill("stop"),"agent/skills/stop/child/SKILL.md":skill("skip-child"),"agent/skills/.hidden/SKILL.md":skill("skip-hidden"),"agent/skills/node_modules/SKILL.md":skill("skip-node"),"repo/.pi/skills/project/SKILL.md":skill("project"),"repo/.agents/skills/ancestor/SKILL.md":skill("ancestor"),"home/.agents/skills/user/SKILL.md":skill("user"),"home/.agents/skills/loose.md":skill("skip-loose")},calls:["/skill:deep"]},
  {name:"collisions",trusted:true,files:{"agent/skills/same/SKILL.md":skill("same","","GLOBAL"),"repo/.pi/skills/same/SKILL.md":skill("same","","PROJECT"),"extra/same/SKILL.md":skill("same","","EXPLICIT")},paths:["../extra","../extra"],calls:["/skill:same"]},
  {name:"symlinks",files:{"agent/skills/a/SKILL.md":skill("linked")},links:{"agent/skills/b":"a","agent/skills/broken":"absent"},paths:["../agent/skills/a/SKILL.md"],calls:["/skill:linked"]},
  {name:"untrusted",files:{"agent/skills/global/SKILL.md":skill("global"),"repo/.pi/skills/secret/SKILL.md":skill("secret"),"repo/.agents/skills/secret/SKILL.md":skill("secret")},calls:["/skill:secret"]},
  {name:"disabled",disabled:true,files:{"agent/skills/global/SKILL.md":skill("global"),"extra/manual/SKILL.md":skill("manual")},paths:["../extra"],calls:["/skill:manual","/skill:global"]},
  {name:"disabled-empty",disabled:true,files:{"agent/skills/global/SKILL.md":skill("global")},calls:["/skill:global"]},
  {name:"explicit-project",files:{"repo/.pi/skills/secret/SKILL.md":skill("secret")},paths:[".pi/skills/secret"],calls:["/skill:secret"]},
  {name:"settings",trusted:true,global:["more","!skip","!**/omit*","+skills/skip","-skills/no"],project:["more"],files:{"agent/more/SKILL.md":skill("same","","GLOBAL_SETTING"),"repo/.pi/more/SKILL.md":skill("same","","PROJECT_SETTING"),"repo/.pi/skills/same/SKILL.md":skill("same","","PROJECT"),"agent/skills/skip/SKILL.md":skill("skip"),"agent/skills/no/SKILL.md":skill("no"),"agent/skills/omit-one/SKILL.md":skill("omit-one")},calls:["/skill:same"]},
  {name:"ignore",files:{"agent/skills/.gitignore":"skip/\n","agent/skills/skip/SKILL.md":skill("skip"),"agent/skills/keep/SKILL.md":skill("keep")},calls:["/skill:keep"]},
  {name:"ancestor-boundary",cwd:"repo/sub/deep",trusted:true,files:{"repo/.git":"gitdir: elsewhere","repo/.agents/skills/root/SKILL.md":skill("root"),"repo/sub/.agents/skills/parent/SKILL.md":skill("parent"),"repo/sub/deep/.agents/skills/child/SKILL.md":skill("child"),".agents/skills/outside/SKILL.md":skill("outside")},calls:["/skill:root"]},
  {name:"ignored-root",files:{"agent/skills/SKILL.md":skill("ignored"),"agent/skills/.gitignore":"SKILL.md\n","agent/skills/root.md":skill("root")},calls:["/skill:root"]},
  {name:"ancestor-settings-priority",trusted:true,global:["more"],files:{"agent/more/SKILL.md":skill("same","","GLOBAL_SETTING"),"repo/.agents/skills/same/SKILL.md":skill("same","","ANCESTOR")},calls:["/skill:same"]},
  {name:"disabled-symlink",global:["skills/a","-skills/a"],files:{"agent/skills/a/SKILL.md":skill("a")},links:{"agent/skills/b":"a"},calls:["/skill:a"]},
  {name:"disabled-symlink-explicit",global:["skills/a","-skills/a"],files:{"agent/skills/a/SKILL.md":skill("a")},links:{"agent/skills/b":"a"},paths:["../agent/skills/b"],calls:["/skill:a"]},
  {name:"explicit-url",files:{"extra/review/SKILL.md":skill("review")},paths:["file://$ROOT/extra/review/SKILL.md"],calls:["/skill:review"]},
 ];
 const outcomes=[];
 for (const scenario of scenarios) {
  const dir=realpathSync(mkdtempSync(join(tmpdir(),"pi-skills-")));
  try {
   for(const [path,content] of Object.entries({...scenario.files,...scenario.extraFiles})){mkdirSync(dirname(join(dir,path)),{recursive:true});writeFileSync(join(dir,path),content);}
   process.env.HOME=join(dir,"home");mkdirSync(process.env.HOME,{recursive:true});
   for(const [p,target] of Object.entries(scenario.links??{})){mkdirSync(dirname(join(dir,p)),{recursive:true});symlinkSync(target,join(dir,p));}
   const cwd=join(dir,scenario.cwd??"repo"),agentDir=join(dir,"agent");mkdirSync(cwd,{recursive:true});mkdirSync(agentDir,{recursive:true});
   writeFileSync(join(agentDir,"settings.json"),JSON.stringify({skills:scenario.global??[]}));
   mkdirSync(join(cwd,".pi"),{recursive:true});writeFileSync(join(cwd,".pi/settings.json"),JSON.stringify({skills:scenario.project??[]}));
   const settings=SettingsManager.create(cwd,agentDir);settings.setProjectTrusted(scenario.trusted??false);
   const loader=new DefaultResourceLoader({cwd,agentDir,settingsManager:settings,noExtensions:true,noSkills:scenario.disabled,noPromptTemplates:true,noThemes:true,noContextFiles:true,additionalSkillPaths:scenario.paths?.map(p=>p.replaceAll("$ROOT",dir))??[]});
   await loader.reload();
   const {skills,diagnostics}=loader.getSkills();
   const normalize=v=>JSON.parse(JSON.stringify(v).replaceAll(dir,"$ROOT"));
   const session={resourceLoader:loader,_extensionRunner:{emitError(){}}};
   outcomes.push({name:scenario.name,skills:normalize(skills),diagnostics:normalize(diagnostics),expanded:normalize(scenario.calls.map(text=>AgentSession.prototype._expandSkillCommand.call(session,text))),systems:[true,false].map(read=>normalize(buildSystemPrompt({cwd,customPrompt:"SYSTEM",selectedTools:read?["read"]:[],skills})))});
  }finally{rmSync(dir,{recursive:true,force:true});}
 }
 const c={schema_version:"1.0.0",id:"sdk/codingagent/skills",catalog_id:"contract:codingagent/skills",surface:"go-sdk",input:{scenarios},observe:["outcome","side_effects"]};
 const observation={outcome:outcomes,side_effects:[]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/skills.mjs <locked-pi-checkout>",platform:"posix",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
}
main().catch(e=>{console.error(e);process.exit(1)});
