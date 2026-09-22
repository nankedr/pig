import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {UserMessageSelectorComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/user-message-selector.js`));
const {TreeSelectorComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/tree-selector.js`));
const {initTheme}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/theme/theme.js`));
const {KeybindingsManager}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/core/keybindings.js`));
const {setKeybindings}=await import(pathToFileURL(`${pi}/packages/tui/dist/index.js`));
initTheme('dark',false);setKeybindings(new KeybindingsManager({}));
const messages=[{id:'u1',text:'first question'},{id:'u2',text:'second question'},{id:'u3',text:'third question'}];
const cases=[{id:'latest',keys:['\r']},{id:'older',keys:['\x1b[A','\r']},{id:'wrap',keys:['\x1b[B','\r']},{id:'initial',initial:'u2',keys:['\r']},{id:'cancel',keys:['\x1b']}];
const outcome=[];
for(const c of cases){let selected='',cancelled=false;
 const s=new UserMessageSelectorComponent(messages,id=>selected=id,()=>cancelled=true,c.initial);
 for(const key of c.keys)s.getMessageList().handleInput(key);
 outcome.push({id:c.id,selected,cancelled});
}
const c={schema_version:'1.0.0',id:'sdk/interactive/fork-selector',catalog_id:'contract:codingagent/interactive-branches',surface:'go-sdk',input:{messages,cases},observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/components/user-message-selector.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/branch-selectors.mjs <locked-pi-checkout>',platform:'any',environment:{sessions:'fixed messages; no network'}};
const path='parity/oracle/fixtures/fork-selector.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);

const timestamp='2026-01-01T00:00:00.000Z';
const entry=(id,parentId,type,rest={})=>({id,parentId,type,timestamp,...rest});
const message=(id,parentId,role,text)=>entry(id,parentId,'message',{message:{role,content:[{type:'text',text}],stopReason:'stop',api:'faux',provider:'faux',model:'test',usage:{input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},toolCallId:'call',toolName:'read',isError:false,timestamp:1767225600000}});
const entries=[entry('m',null,'model_change',{provider:'faux',modelId:'test'}),message('u1','m','user','first question'),message('a1','u1','assistant','first answer'),message('u2','a1','user','old branch question'),message('a2','u2','assistant','old branch answer'),message('u3','a1','user','active question'),message('tool','u3','toolResult','tool output'),message('a3','tool','assistant','active answer'),entry('label','a3','label',{targetId:'a2',label:'bookmark'}),entry('info','label','session_info',{name:'named session'})];
const treeCases=[
 {id:'current',keys:['\r']},{id:'all-leaf',keys:['\x01','\r']},{id:'users',keys:['\x15','\r']},{id:'label',keys:['\x0c','\r']},{id:'search',keys:['old answer','\r']},{id:'clear-search',keys:['missing','\x1b','\r']},{id:'cancel-tree',keys:['\x1b']},
 {id:'page-first',keys:['\x1b[5~','\r']},{id:'page-last',keys:['\x1b[6~','\r']},{id:'wrap-tree',initial:'u1',keys:['\x1b[A','\r']},{id:'cycle',keys:['\x0f','\x0f','\r']},{id:'toggle-users',keys:['\x15','\x15','\r']},{id:'label-search',keys:['bookmark','\r']},{id:'nearest-parent',initial:'m',keys:['\r']},
 {id:'fold-root',initial:'u1',keys:['\x1b[1;5D','\x1b[B','\r']},{id:'unfold-root',initial:'u1',keys:['\x1b[1;5D','\x1b[1;5C','\x1b[B','\r']},{id:'segment-up',keys:['\x1b[1;5D','\r']},{id:'segment-down',initial:'u1',keys:['\x1b[1;5C','\r']},
 {id:'set-label',initial:'a1',keys:['L','checkpoint','\r','\x0c','\r']},{id:'remove-label',initial:'a2',keys:['L','\x05',...Array(8).fill('\x7f'),'\r','\x0c','\r']},{id:'cancel-label',initial:'a1',keys:['L','draft','\x1b','\r']},{id:'copy',initial:'a1',keys:['\x18','\r']},
 {id:'no-tools',initial:'tool',keys:['\x14','\r']},{id:'initial-mode',mode:'user-only',keys:['\r']},
];
const treeOutcome=[];
for(const tc of treeCases){
 const nodes=new Map(entries.map(e=>[e.id,{entry:structuredClone(e),children:[]}]));
 const roots=[];for(const node of nodes.values()){if(node.entry.parentId)nodes.get(node.entry.parentId).children.push(node);else roots.push(node);if(node.entry.type==='label')nodes.get(node.entry.targetId).label=node.entry.label;}
 let selected='',cancelled=false;const labels=[],copies=[];
 const s=new TreeSelectorComponent(roots,'info',20,id=>selected=id,()=>cancelled=true,(id,label)=>labels.push({id,label:label??null}),tc.initial,tc.mode);
 s.onCopy=text=>copies.push(text??'');
 for(const key of tc.keys)s.handleInput(key);
 treeOutcome.push({id:tc.id,selected,cancelled,labels,copies});
}
const treeCase={schema_version:'1.0.0',id:'sdk/interactive/tree-selector',catalog_id:'contract:codingagent/interactive-branches',surface:'go-sdk',input:{entries,cases:treeCases,leaf:'info'},observe:['outcome','side_effects']};
const treeObservation={outcome:treeOutcome,side_effects:[]};
const treeFixture={...fixture,upstream:{...fixture.upstream,reference:'packages/coding-agent/src/modes/interactive/components/tree-selector.ts'},case:treeCase,observation:treeObservation,input_hash:caseDigest(treeCase),observation_hash:observationDigest(treeObservation)};
const treePath='parity/oracle/fixtures/tree-selector.json';
if(process.argv.includes('--check')){if(JSON.stringify(treeFixture)!==JSON.stringify(JSON.parse(readFileSync(treePath))))throw Error('tree fixture drift');}else writeFileSync(treePath,JSON.stringify(treeFixture,null,2)+'\n');
console.log(treeOutcome);
