import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {SessionSelectorComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/session-selector.js`));
const {initTheme}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/theme/theme.js`));
initTheme("dark",false);
const {KeybindingsManager}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/core/keybindings.js`));
const {setKeybindings}=await import(pathToFileURL(`${pi}/packages/tui/dist/index.js`));
setKeybindings(new KeybindingsManager({}));
const sessions=[
 {path:'/sessions/child.jsonl',id:'child',cwd:'/project',parentSessionPath:'/sessions/root.jsonl',name:'Child task',firstMessage:'fix node cve',allMessagesText:'fix node cve regression',messageCount:2,created:'2026-01-01T00:00:00.000Z',modified:'2026-01-05T00:00:00.000Z'},
 {path:'/sessions/other.jsonl',id:'other',cwd:'/other',name:'Other project',firstMessage:'work elsewhere',allMessagesText:'work elsewhere',messageCount:2,created:'2026-01-01T00:00:00.000Z',modified:'2026-01-04T00:00:00.000Z'},
 {path:'/sessions/plain.jsonl',id:'plain',cwd:'/project',firstMessage:'node stuff cve',allMessagesText:'node stuff cve',messageCount:2,created:'2026-01-01T00:00:00.000Z',modified:'2026-01-03T00:00:00.000Z'},
 {path:'/sessions/root.jsonl',id:'root',cwd:'/project',name:'Root task',firstMessage:'fix bug',allMessagesText:'fix bug original',messageCount:2,created:'2026-01-01T00:00:00.000Z',modified:'2026-01-02T00:00:00.000Z'},
];
const cases=[{id:'threaded',keys:['\r']},{id:'recent',keys:['\x13','\r']},{id:'all',keys:['\t','other','\r']},{id:'phrase',keys:['"node cve"','\r']},{id:'tokens',keys:['node cve','\r']},{id:'regex',keys:['re:node.*cve','\r']},{id:'invalid',keys:['re:[','\r','\x1b']},{id:'named',keys:['\x0e','\x1b[B','\r']},{id:'clamp',keys:['\x1b[A','\r']},{id:'cancel',keys:['\x1b']},{id:'sort-cycle',keys:['\x13','\x13','\x13','\r']},{id:'page',keys:['\x1b[6~','\r']},{id:'scope-back',keys:['\t','\t','\r']},{id:'path',keys:['\x10','\r']},{id:'unclosed',keys:['"node','\r','\x1b']}];
const outcome=[];
for(const c of cases){let selected='',cancelled=false;
 const entries=sessions.map(s=>({...s,created:new Date(s.created),modified:new Date(s.modified)}));
 const selector=new SessionSelectorComponent(async()=>entries.filter(s=>s.cwd==='/project'),async()=>entries,p=>selected=p,()=>cancelled=true,()=>{},()=>{});
 await new Promise(r=>setTimeout(r,0));
 for(const key of c.keys){selector.handleInput(key);await new Promise(r=>setTimeout(r,0));}
 outcome.push({id:c.id,selected,cancelled});
}
const c={schema_version:'1.0.0',id:'sdk/interactive/session-selector',catalog_id:'contract:codingagent/session-selection',surface:'go-sdk',input:{sessions,cases},observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/components/session-selector.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/session-selector.mjs <locked-pi-checkout>',platform:'any',environment:{sessions:'fixed discovery snapshot; no network'}};
const path='parity/oracle/fixtures/session-selector.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);
