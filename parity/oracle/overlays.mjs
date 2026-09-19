import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {TuiMainScreen}=await import(pathToFileURL(`${pi}/packages/tui/dist/tui-main-screen.js`));
const terminal={columns:20,rows:6,start(){},stop(){},write(){},hideCursor(){},showCursor(){},kittyProtocolActive:false};
const ui=new TuiMainScreen(terminal);
const base={render:()=>Array(6).fill('abcdefghijklmnopqrst'),invalidate(){}};
ui.addChild(base);ui.setFocus(base);ui.start();
const cases=[
 {name:'center',options:{width:8},lines:['hello','world']},
 {name:'top-right',options:{width:8,anchor:'top-right'},lines:['hello']},
 {name:'percent-margin',options:{width:'50%',row:'100%',col:'100%',margin:1,maxHeight:2},lines:['one','two','three']},
 {name:'wide-cell',options:{width:5,row:0,col:1},lines:['中界hello']},
 {name:'clipped-offset',options:{width:40,minWidth:30,anchor:'bottom-left',offsetX:50,offsetY:10},lines:['hello']},
];
const clean=s=>s.replace(/\x1b\][^\x07]*\x07/g,'').replace(/\x1b\[[0-?]*[ -/]*[@-~]/g,'').trimEnd();
const frames=[];
for(const c of cases){const handle=ui.showOverlay({render:()=>c.lines,invalidate(){}},c.options);ui.renderNow();frames.push(ui.captureRenderState().previousLines.map(clean));handle.hide();ui.renderNow();}
ui.stop();
const c={schema_version:'1.0.0',id:'tui/overlays',catalog_id:'contract:tui/dialogs-trust',surface:'go-sdk',input:{width:20,height:6,cases},observe:['outcome','side_effects']};
const observation={outcome:{frames},side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/tui.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/overlays.mjs <locked-pi-checkout>',platform:'any',environment:{transport:'public TuiMainScreen renderNow/captureRenderState'}};
const path='parity/oracle/fixtures/overlays.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
