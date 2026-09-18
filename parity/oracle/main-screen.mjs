import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]);
const lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const {TuiMainScreen}=await import(pathToFileURL(`${pi}/packages/tui/dist/tui-main-screen.js`));
const marker='\x1b_pi:c\x07';
const initial=Array.from({length:9},(_,i)=>`row ${i}`);
const steps=[
 {name:'initial',lines:[...initial,`> ab${marker}c`]},
 {name:'edit',lines:[...initial,`> abX${marker}c`]},
 {name:'cursor',lines:[...initial,`> a${marker}bXc`]},
 {name:'append',lines:[...initial,'stream 1','stream 2',`> a${marker}bXc`]},
 {name:'delete-visible',lines:[...initial,'stream 1']},
 {name:'append-blank',lines:[...initial,'stream 1','','']},
 {name:'rewrite-history',lines:['changed',...initial.slice(1),'stream 1','',`> ${marker}`]},
 {name:'resize-width',width:14,lines:['中文 👩‍💻','changed',...initial,`> ${marker}`]},
 {name:'resize-height',height:7},
 {name:'shrink',lines:['short',`> ${marker}`]},
 {name:'normalized-width',lines:['a\tb',`> ${marker}`]},
 {name:'delete-all',lines:[]},
 {name:'restart',lines:['after',`> ${marker}`]},
 {name:'force',force:true},
 {name:'clear-on-shrink',clearOnShrink:true,lines:[`> ${marker}`]},
];
let output='';
const terminal={columns:20,rows:5,start(){},stop(){},write(s){output+=s},hideCursor(){output+='\x1b[?25l'},showCursor(){output+='\x1b[?25h'},kittyProtocolActive:false};
const ui=new TuiMainScreen(terminal,true);
let lines=[];
ui.addChild({render:()=>[...lines],invalidate(){}});
const observations=[];
for(const [i,step] of steps.entries()){
 output=''; if(step.lines)lines=step.lines; if(step.width)terminal.columns=step.width;if(step.height)terminal.rows=step.height;
 if(step.clearOnShrink)ui.setClearOnShrink(true);
 if(i===0)ui.start();
 ui.renderNow(step.force);
 observations.push({output,state:ui.captureRenderState(),fullRedraws:ui.fullRedraws});
}
output='';ui.stop();const stopOutput=output;
const {TuiAltScreen}=await import(pathToFileURL(`${pi}/packages/tui/dist/tui-alt-screen.js`));
const fullscreenSteps=[{name:'initial',lines:['heading',`> ab${marker}c`]},{name:'edit',lines:['heading',`> abX${marker}c`]},{name:'cursor',lines:['heading',`> a${marker}bXc`]},{name:'delete',lines:[`> ${marker}`]},{name:'resize',width:12,height:3}];
terminal.columns=20;terminal.rows=5;
const alt=new TuiAltScreen(terminal,true);alt.addChild({render:()=>[...lines],invalidate(){}});
const fullscreen=[];
for(const [i,step] of fullscreenSteps.entries()){
 output='';if(step.lines)lines=step.lines;if(step.width)terminal.columns=step.width;if(step.height)terminal.rows=step.height;
 if(i===0)alt.start();alt.renderNow();fullscreen.push({output,fullRedraws:alt.fullRedraws});
}
alt.stop({preserveScreen:true});
const c={schema_version:'1.0.0',id:'tui/main-screen',catalog_id:'contract:tui/layout-scrolling',surface:'go-sdk',input:{width:20,height:5,steps,fullscreenSteps},observe:['outcome','side_effects']};
const observation={outcome:{steps:observations,stopOutput,fullscreen},side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/tui-main-screen.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/main-screen.mjs <locked-pi-checkout>',platform:'any',environment:{TERMUX_VERSION:'unset'}};
const path='parity/oracle/fixtures/main-screen.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
