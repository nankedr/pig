import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]);
const lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {Markdown}=await import(pathToFileURL(`${pi}/packages/tui/dist/components/markdown.js`));
const {stripTerminalSequences,visibleWidth,wrapTextWithAnsi,truncateToWidth}=await import(pathToFileURL(`${pi}/packages/tui/dist/utils.js`));
const {AssistantMessageComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/assistant-message.js`));
const {renderDiff}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/diff.js`));
const {initTheme}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/theme/theme.js`));
initTheme("dark",false);
const identity=s=>s;
const theme=Object.fromEntries(['heading','link','linkUrl','code','codeBlock','codeBlockBorder','quote','quoteBorder','hr','listBullet','bold','italic','strikethrough','underline'].map(k=>[k,identity]));
const markdown=[
 {text:"- item\n\n  ```go\n  println(1)\n  ```",width:40},
 {text:'# Title\n\nHello **bold** and *italic* with `code`.\n\n[site](https://example.com)\n\n- one\n- two',width:70},
 {text:'```go\nfunc main() {\n\tprintln("你好")\n}\n```',width:26},
 {text:'3. alpha beta gamma delta\n4. 中文👨‍👩‍👧‍👦 emoji\n   - nested\n   - second',width:17},
 {text:'> quote **bold**\n> next\n\n---\n\n~~deleted~~',width:20},
 {text:'partial\n\n```go\nhello',width:16},
 {text:'中👨‍👩‍👧‍👦é supercalifragilistic',width:7},
 {text:'7) literal\n9) marker\n\n\\*literal\\*',width:30,options:{preserveOrderedListMarkers:true,preserveBackslashEscapes:true}},
];
const assistant=[
 {content:[{type:'thinking',thinking:'reason one'},{type:'thinking',thinking:'reason two'},{type:'text',text:'**answer**'}],stopReason:'stop'},
 {content:[{type:'text',text:'partial answer'}],stopReason:'aborted'},
 {content:[{type:'text',text:'partial answer'}],stopReason:'error',errorMessage:'broken'},
 {content:[{type:'text',text:'partial answer'}],stopReason:'length'},
];
const clean=lines=>lines.map(s=>stripTerminalSequences(s).trimEnd());
const input={markdown,assistant};
const outcome={markdown:markdown.map(c=>clean(new Markdown(c.text,0,0,theme,undefined,c.options).render(c.width))),assistant:assistant.map(m=>[false,true].map(h=>clean(new AssistantMessageComponent(m,h,theme,'Thinking...',0).render(50)))),diff:stripTerminalSequences(renderDiff('-1 old\tvalue\n+1 new\tvalue\n 2 context'))};
const c={schema_version:'1.0.0',id:'codingagent/text-rendering',catalog_id:'contract:codingagent/text-rendering',surface:'go-sdk',input,observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/components/assistant-message.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/text-rendering.mjs <locked-pi-checkout>',platform:'any',environment:{colors:'stripped',hyperlinks:false}};
const path='parity/oracle/fixtures/text-rendering.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(JSON.stringify(outcome,null,2));
