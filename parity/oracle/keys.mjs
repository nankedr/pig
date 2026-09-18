import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { caseDigest, observationDigest } from './fixture-hash.mjs';
const pi = resolve(process.argv[2]);
const lock = JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if (execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim()) throw Error('dirty baseline');
const keys = await import(pathToFileURL(`${pi}/packages/tui/dist/keys.js`));
const {StdinBuffer} = await import(pathToFileURL(`${pi}/packages/tui/dist/stdin-buffer.js`));
const {KeybindingsManager,TUI_KEYBINDINGS} = await import(pathToFileURL(`${pi}/packages/tui/dist/keybindings.js`));
const cases = JSON.parse(readFileSync('parity/terminal/keys-cases.json'));
const parsed = [false,true].map(active => {
 keys.setKittyProtocolActive(active);
 return cases.data.map(data => ({key:keys.parseKey(data)??null, printable:keys.decodePrintableKey(data)??null, kittyPrintable:keys.decodeKittyPrintable(data)??null, release:keys.isKeyRelease(data),repeat:keys.isKeyRepeat(data),matches:cases.keys.filter(k=>keys.matchesKey(data,k))}));
});
const buffers = cases.buffers.map(chunks=>{
 const b=new StdinBuffer({timeout:10000}), events=[];
 b.on('data',s=>events.push(['data',s])); b.on('paste',s=>events.push(['paste',s]));
 for(const s of chunks)b.process(s);
 const remainder=b.getBuffer(), flushed=b.flush();b.destroy();
 return {events,remainder,flushed};
});
const manager=new KeybindingsManager(TUI_KEYBINDINGS,cases.bindings);
const resolved=manager.getResolvedBindings(); for(const k in resolved) if(typeof resolved[k]==='string')resolved[k]=[resolved[k]];
const conflicts=manager.getConflicts().map(c=>({...c,keybindings:c.keybindings.sort()})).sort((a,b)=>a.key.localeCompare(b.key));
const c={schema_version:'1.0.0',id:'tui/terminal/keys',catalog_id:'contract:tui/terminal-keys',surface:'go-sdk',input:cases,observe:['outcome','side_effects']};
const {Editor}=await import(pathToFileURL(`${pi}/packages/tui/dist/components/editor.js`));
const {setKeybindings}=await import(pathToFileURL(`${pi}/packages/tui/dist/keybindings.js`));
setKeybindings(new KeybindingsManager(TUI_KEYBINDINGS,cases.editor.config));
const editor=new Editor({terminal:{rows:32,columns:80},requestRender(){}},{borderColor:s=>s,selectList:{}});
const submits=[], states=[];editor.onSubmit=s=>{submits.push(s);editor.addToHistory(s)};
for(const step of cases.editor.steps){editor.handleInput(step);states.push(editor.getText());}
const observation={outcome:{parsed,buffers,resolved,conflicts,editor:{states,submits}},side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/keys.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/keys.mjs <locked-pi-checkout>',platform:'darwin',environment:{buffer:'UTF-8 decoded strings; byte fragmentation tested independently'}};
const path='parity/oracle/fixtures/keys.json';
if(process.argv.includes('--check')) {if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path)))) throw Error('fixture drift');} else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(`captured ${cases.data.length} key encodings in two modes`);
