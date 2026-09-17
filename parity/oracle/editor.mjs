import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { caseDigest, observationDigest } from './fixture-hash.mjs';
const pi = resolve(process.argv[2]);
const lock = JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if (execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim()) throw Error('dirty baseline');
const {Editor} = await import(pathToFileURL(`${pi}/packages/tui/dist/components/editor.js`));
const cases = JSON.parse(readFileSync('parity/terminal/editor-cases.json'));
const outcomes = cases.map(c => {
 const editor = new Editor({terminal:{rows:32,columns:40},requestRender(){}},{borderColor:s=>s,selectList:{}});
 const changes=[], submits=[], states=[];
 editor.onChange = text=>changes.push(text);
 editor.onSubmit = text=>submits.push(text);
 for (const step of c.steps) {
  if(step.op==='input') editor.handleInput(step.text);
  if(step.op==='set') editor.setText(step.text);
  if(step.op==='insert') editor.insertTextAtCursor(step.text);
  if(step.op==='history') editor.addToHistory(step.text);
  if(step.op==='focus') editor.focused=step.value;
  if(step.op==='disable') editor.disableSubmit=step.value;
  const frame = step.op==='render' ? editor.render(step.width) : undefined;
  const cursor=editor.getCursor();
  states.push({frame,text:editor.getText(),expanded:editor.getExpandedText(),cursor:{Line:cursor.line,Col:Buffer.byteLength(editor.getLines()[cursor.line].slice(0,cursor.col))}});
 }
 return {id:c.id,states,changes,submits};
});
const c={schema_version:'1.0.0',id:'tui/editor/multiline',catalog_id:'contract:tui/multiline-editor',surface:'go-sdk',input:cases,observe:['outcome','side_effects']};
const observation={outcome:outcomes,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/components/editor.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/editor.mjs <locked-pi-checkout>',platform:'any',environment:{cursor:'UTF-8 byte offsets projected from Pi UTF-16 offsets'}};
const path='parity/oracle/fixtures/editor.json';
if(process.argv.includes('--check')) {if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path)))) throw Error('fixture drift');} else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(`captured ${cases.length} editor cases`);
