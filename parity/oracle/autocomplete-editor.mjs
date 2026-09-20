import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {CombinedAutocompleteProvider}=await import(pathToFileURL(`${pi}/packages/tui/dist/autocomplete.js`));
const {Editor}=await import(pathToFileURL(`${pi}/packages/tui/dist/components/editor.js`));
const cases=[
{id:'second-line-no-command-menu',text:'draft\n',steps:['/rev','\t','\r']},
{id:'select-tab',steps:['/','\x1b[B','\t']},
{id:'cancel-draft',steps:['/r','\x1b[B','\x1b']},
{id:'enter-command-submits',steps:['/rev','\r']},
{id:'accept-undo',steps:['/rev','\t','\x1f']},
{id:'empty-result-retrigger',steps:['/','zzzz','\x7f','\x7f','\x7f','\x7f']},
{id:'middle-replace',text:'/re suffix',steps:['\x01','\x1b[C','\x1b[C','\x1b[C','\t','\t']},
{id:'unicode-command',steps:['/中文','\t']},
{id:'typing-closes-menu',steps:['/rev',' ','arg','\r']},
{id:'paste-no-trigger',steps:['\x1b[200~/rev\x1b[201~','\r']},
];
const outcomes=[];
for(const c of cases){
 const e=new Editor({terminal:{rows:32,columns:60},requestRender(){}},{borderColor:s=>s,selectList:{selectedPrefix:s=>s,selectedText:s=>s,description:s=>s,scrollInfo:s=>s,noMatch:s=>s}});
 e.setAutocompleteProvider(new CombinedAutocompleteProvider([{name:'review'},{name:'reload'},{name:'resume'},{name:'skill:中文'}],'/nonexistent',null));
 if(c.text)e.setText(c.text);
 const submits=[],states=[],changes=[];e.onSubmit=text=>submits.push(text);e.onChange=text=>changes.push(text);
 for(const input of c.steps){e.handleInput(input);await new Promise(r=>setTimeout(r,120));const cursor=e.getCursor();states.push({text:e.getText(),showing:e.isShowingAutocomplete(),cursor:{Line:cursor.line,Col:Buffer.byteLength(e.getLines()[cursor.line].slice(0,cursor.col))}});}
 outcomes.push({id:c.id,states,submits,changes});
}
const c={schema_version:'1.0.0',id:'tui/autocomplete/editor',catalog_id:'contract:tui/autocomplete',surface:'go-sdk',input:cases,observe:['outcome','side_effects']};
const observation={outcome:outcomes,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/components/editor.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/autocomplete-editor.mjs <locked-pi-checkout>',platform:'any',environment:{cursor:'UTF-8 byte offsets',filesystem:'absent'}};
const path='parity/oracle/fixtures/autocomplete-editor.json';if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(`captured ${cases.length} editor completion cases`);
