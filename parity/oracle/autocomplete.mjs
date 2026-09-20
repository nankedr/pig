import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdtempSync,mkdirSync,rmSync,chmodSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {resolve,join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {CombinedAutocompleteProvider}=await import(pathToFileURL(`${pi}/packages/tui/dist/autocomplete.js`));
const commands=[{name:'review',description:'Review changes',argumentHint:'<file>'},{name:'reload'},{name:'resume'},{name:'skill:中文'},{name:'skill:review'},{name:'model',getArgumentCompletions:async prefix=>prefix==='d'?[{value:'deepseek/model',label:'model'}]:null}];
const files=['alpha.txt','a space.txt','中文.txt','src/main.go','space dir/inner.txt'];
const fdScript="#!/bin/sh\n[ \"$1\" = --base-directory ] || exit 1\ncase \"$2\" in */src/) printf 'main.go\\n';; */src) printf 'main.go\\n';; *) printf 'src/\\nsrc/main.go\\nalpha.txt\\na space.txt\\nspace dir/\\n.git/config\\n' ;; esac\n";
const cases=[
{id:'fd-file',text:'@alpha',fd:true},{id:'fd-spaces',text:'@space',fd:true},{id:'fd-scope',text:'@src/ma',fd:true},{id:'fd-directory',text:'@src',fd:true},{id:'fd-quoted-middle',text:'read @"a sp" tail',col:11,fd:true},{id:'fd-missing',text:'@alpha',fd:'missing'},
{id:'commands',text:'/'},{id:'fuzzy',text:'/rv'},{id:'unicode',text:'/中文'},{id:'empty',text:'/zzzz'},
{id:'middle-command',text:'/rev suffix',col:4},{id:'argument',text:'/model d'},
{id:'plain-no-trigger',text:'alp'},{id:'forced-file',text:'alp',force:true},
{id:'directory',text:'./s'},{id:'quoted',text:'read "a sp" untouched',col:10,force:true},
{id:'unicode-path',text:'看 ./中'},{id:'directory-space',text:'"space d',force:true},
{id:'no-fd',text:'@alpha'},{id:'no-fd-forced',text:'@alpha',force:true},
{id:'absolute',text:'${ROOT}/alp',force:true},{id:'relative-parent',text:'src/../alp'},
{id:'missing-directory',text:'./missing/'},{id:'ordinary-slash-argument',text:'/review x'},
];
const root=mkdtempSync(join(tmpdir(),'autocomplete-'));
let outcomes;
try{
 for(const f of files){mkdirSync(join(root,f,'..'),{recursive:true});writeFileSync(join(root,f),'test');}
 const fdPath=join(root,'fd-tool');writeFileSync(fdPath,fdScript);chmodSync(fdPath,0o755);
 outcomes=await Promise.all(cases.map(async c=>{
 const provider=new CombinedAutocompleteProvider(commands,root,c.fd==='missing'?join(root,'missing-fd'):c.fd?fdPath:null);
 const text=c.text.replaceAll('${ROOT}',root),col=c.col??text.length;
 const suggestions=await provider.getSuggestions([text],0,col,{signal:new AbortController().signal,force:c.force??false});
 const application=suggestions?provider.applyCompletion([text],0,col,suggestions.items[0],suggestions.prefix):null;
 if(application)application.cursorCol=Buffer.byteLength(application.lines[0].slice(0,application.cursorCol));
 return {id:c.id,suggestions,application,trigger:provider.shouldTriggerFileCompletion([text],0,col)};
 }));
 outcomes=JSON.parse(JSON.stringify(outcomes).replaceAll(root,'${ROOT}'));
 // Project absolute cursor offsets to the stable fixture placeholder.
 for(const o of outcomes)if(o.application&&o.application.lines[0].includes('${ROOT}'))o.application.cursorCol+=7-root.length;
}finally{rmSync(root,{recursive:true,force:true});}
const c={schema_version:'1.0.0',id:'tui/autocomplete/provider',catalog_id:'contract:tui/autocomplete',surface:'go-sdk',input:{files,cases,fdScript},observe:['outcome','side_effects']};
const observation={outcome:outcomes,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/autocomplete.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/autocomplete.mjs <locked-pi-checkout>',platform:'any',environment:{cursor:'UTF-8 byte offsets',root:'${ROOT}',fd:'controlled executable or unavailable'}};
const path='parity/oracle/fixtures/autocomplete.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(`captured ${cases.length} autocomplete cases`);
