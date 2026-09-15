import { caseDigest, observationDigest } from "./fixture-hash.mjs";
import { execFileSync } from "node:child_process";
import { realpathSync, symlinkSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "themes.json");

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
 const args=parseArgs(process.argv);
 const lock=JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json"),"utf8"));
 if(execFileSync("git",["-C",args.pi,"rev-parse","HEAD"],{encoding:"utf8"}).trim()!==lock.upstream.commit)throw new Error("baseline mismatch");
 process.env.COLORTERM="truecolor";process.env.FORCE_COLOR="3";
 const reference="packages/coding-agent/src/modes/interactive/theme/theme.ts";
 const api=await import(pathToFileURL(join(args.pi,reference)));
 const {DefaultResourceLoader}=await import(pathToFileURL(join(args.pi,"packages/coding-agent/src/core/resource-loader.ts")));
 const {SettingsManager}=await import(pathToFileURL(join(args.pi,"packages/coding-agent/src/core/settings-manager.ts")));
 const {exportFromFile}=await import(pathToFileURL(join(args.pi,"packages/coding-agent/src/core/export-html/index.ts")));
 const dark=JSON.parse(readFileSync(join(args.pi,dirname(reference),"dark.json"),"utf8"));
 const light=JSON.parse(readFileSync(join(args.pi,dirname(reference),"light.json"),"utf8"));
 const custom=structuredClone(dark);custom.name="custom";custom.vars.chain="accent";custom.colors.accent="chain";custom.colors.text="";custom.colors.border=196;delete custom.colors.thinkingMax;delete custom.colors.scrollbarThumb;custom.export={pageBg:"#123456",cardBg:235,infoBg:""};
 const documents=[dark,light,custom];
 const outcomes=[];
 const dir=realpathSync(mkdtempSync(join(tmpdir(),"pi-themes-")));
 try {
 for(const doc of documents){
  const path=join(dir,doc.name+".json");writeFileSync(path,JSON.stringify(doc));
  const themes=["truecolor","256color"].map(mode=>api.loadThemeFromPath(path,mode));api.setRegisteredThemes([themes[0]]);
  outcomes.push({name:doc.name});
  outcomes.at(-1).modes=themes.map(t=>({mode:t.getColorMode(),fg:Object.fromEntries([...t.fgColors.keys()].map(k=>[k,t.fg(k,"X")])),bg:Object.fromEntries([...t.bgColors.keys()].map(k=>[k,t.bg(k,"X")])),thinking:["off","minimal","low","medium","high","xhigh","max","other"].map(k=>t.getThinkingBorderColor(k)("X")),styles:[t.bold("X"),t.italic("X"),t.underline("X"),t.inverse("X"),t.strikethrough("X")]}));
  outcomes.at(-1).css=api.getResolvedThemeColors(doc.name);outcomes.at(-1).export=api.getThemeExportColors(doc.name);
  const source=join(dir,"session.jsonl"),output=join(dir,"export.html");writeFileSync(source,readFileSync(join(root,"parity/export-html/session.jsonl")));
  await exportFromFile(source,{outputPath:output,themeName:doc.name});
  const html=readFileSync(output,"utf8");outcomes.at(-1).html=Object.fromEntries([...html.matchAll(/--([A-Za-z-]+): ([^;]+);/g)].filter(m=>m[1] in outcomes.at(-1).css||["body-bg","container-bg","info-bg","exportPageBg","exportCardBg","exportInfoBg"].includes(m[1])).map(m=>[m[1],m[2]]));
 }
 const invalid=[{name:"missing",doc:{name:"bad",colors:{}}},{name:"cycle",doc:{...dark,vars:{...dark.vars,accent:"loop",loop:"accent"}}},{name:"unknown",doc:{...dark,colors:{...dark.colors,accent:"absent"}}},{name:"index",doc:{...dark,colors:{...dark.colors,accent:256}}},{name:"hex",doc:{...dark,colors:{...dark.colors,accent:"#fff"}}},{name:"slash",doc:{...dark,name:"light/dark"}},{name:"type",doc:{...dark,colors:{...dark.colors,accent:true}}}];
 const errors=invalid.map(c=>{const path=join(dir,"bad.json");writeFileSync(path,JSON.stringify(c.doc));try{api.loadThemeFromPath(path);return false}catch{return true}});
 const theme=(name)=>JSON.stringify({...dark,name});
 const scenarios=[
 {name:"empty",files:{}},
 {name:"discovery",trusted:true,files:{"agent/themes/global.json":theme("global"),"repo/.pi/themes/project.json":theme("project"),"agent/themes/nested/skip.json":theme("skip"),"agent/themes/plain.txt":"TEXT"}},
 {name:"collision",trusted:true,files:{"agent/themes/same.json":theme("same"),"repo/.pi/themes/same.json":theme("same"),"extra/same.json":theme("same")},paths:["../extra","../extra"]},
 {name:"untrusted",files:{"agent/themes/global.json":theme("global"),"repo/.pi/themes/project.json":theme("project")}},
 {name:"disabled",disabled:true,files:{"agent/themes/global.json":theme("global"),"extra/custom.json":theme("custom")},paths:["../extra"]},
 {name:"explicit-project",files:{"repo/.pi/themes/project.json":theme("project")},paths:[".pi/themes/project.json"]},
 {name:"missing",files:{"plain.txt":"TEXT"},paths:["../absent","../plain.txt"]},
 {name:"settings",trusted:true,global:["more","!**/skip.json"],project:["more"],files:{"agent/more/a.json":theme("same"),"repo/.pi/more/a.json":theme("same"),"agent/themes/skip.json":theme("skip"),"agent/themes/keep.json":theme("keep")}},
 ];
 const resources=[];
 for(const scenario of scenarios){
  const folder=join(dir,scenario.name);mkdirSync(folder,{recursive:true});
  for(const [p,v] of Object.entries(scenario.files)){mkdirSync(dirname(join(folder,p)),{recursive:true});writeFileSync(join(folder,p),v)}
  const cwd=join(folder,"repo"),agentDir=join(folder,"agent");mkdirSync(join(cwd,".pi"),{recursive:true});mkdirSync(agentDir,{recursive:true});
  writeFileSync(join(agentDir,"settings.json"),JSON.stringify({themes:scenario.global??[]}));writeFileSync(join(cwd,".pi/settings.json"),JSON.stringify({themes:scenario.project??[]}));
  const settings=SettingsManager.create(cwd,agentDir);settings.setProjectTrusted(scenario.trusted??false);
  const loader=new DefaultResourceLoader({cwd,agentDir,settingsManager:settings,noExtensions:true,noSkills:true,noPromptTemplates:true,noContextFiles:true,noThemes:scenario.disabled,additionalThemePaths:scenario.paths??[]});await loader.reload();
  const normalize=v=>JSON.parse(JSON.stringify(v).replaceAll(folder,"$ROOT"));
  const loaded=loader.getThemes();resources.push({name:scenario.name,themes:normalize(loaded.themes.map(t=>({name:t.name,path:t.sourcePath,source:t.sourceInfo}))),diagnostics:normalize(loaded.diagnostics)});
 }
 const c={schema_version:"1.0.0",id:"sdk/codingagent/themes",catalog_id:"contract:codingagent/themes",surface:"go-sdk",input:{documents,invalid,scenarios},observe:["outcome","side_effects"]};
 const observation={outcome:{colors:outcomes,errors,resources},side_effects:[]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/themes.mjs <locked-pi-checkout>",platform:"posix",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 } finally{rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
