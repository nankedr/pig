import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]);
const lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
const load=p=>import(pathToFileURL(`${pi}/packages/tui/dist/${p}.js`));
const {ScrollView}=await load('components/scroll-view');
const {VStack}=await load('components/v-stack');
const {HStack}=await load('components/h-stack');
const {renderLayoutFrame,getScrollViewsAt,getScrollViewBox,getScrollbarGeometry}=await load('layout');
const {allocateStackSizes}=await load('components/stack');
const {stripTerminalSequences}=await load('utils');
const content={lines:Array.from({length:12},(_,i)=>`row ${i}`),render(){return this.lines},invalidate(){}};
const footer={render:()=>['input','cursor\x1b_pi:c\x07'],invalidate(){}};
const scroll=new ScrollView(content,{follow:'end',primary:true,scrollbar:'always'});
const root=new VStack([{component:scroll,basis:0,grow:1},{component:footer,shrink:0}]);
const states=[];
const record=(width,height)=>{const frame=renderLayoutFrame(root,width,height,()=>{});states.push({lines:frame.lines.map(l=>stripTerminalSequences(l).trimEnd()),top:scroll.scrollTop,following:scroll.isFollowingEnd,viewport:scroll.viewportHeight,geometry:getScrollbarGeometry(getScrollViewBox(frame,scroll))});};
record(12,7);scroll.scrollBy(-3);record(12,7);content.lines.push('row 12','row 13');record(12,7);record(8,6);scroll.scrollToEnd();record(8,6);content.lines=['short'];record(8,6);scroll.scrollToStart();record(8,3);
const allocation=[{entries:[{basis:8,shrink:1},{basis:6,shrink:1}],intrinsic:[8,6],available:8,gap:1},{entries:[{basis:0,grow:1,maxSize:3},{basis:0,grow:2}],intrinsic:[0,0],available:11,gap:1},{entries:[{minSize:4},{minSize:2}],intrinsic:[10,8],available:3,gap:0}];
const inner=new ScrollView({render:()=>['a','b','c','d','e'],invalidate(){}},{scrollbar:'always'});
const outer=new ScrollView(new VStack([{component:inner,basis:3},{component:footer,basis:6}]),{primary:true});
const nested=renderLayoutFrame(outer,12,5,()=>{});
const horizontal=renderLayoutFrame(new HStack([{component:footer,basis:5},{component:content,grow:1}],{gap:1,align:'end'}),12,4,()=>{});
const {TuiAltScreen}=await load('tui-alt-screen');
const mouse=[];
for(const overscroll of ['chain','contain']) {
 let onInput;
 const terminal={columns:12,rows:5,start(input){onInput=input},stop(){},write(){},hideCursor(){},showCursor(){},kittyProtocolActive:false};
 const inner=new ScrollView({render:()=>['a','b','c','d','e'],invalidate(){}},{scrollbar:'always',overscroll});
 const outer=new ScrollView(new VStack([{component:inner,basis:3},{component:footer,basis:6}]),{primary:true});
 const ui=new TuiAltScreen(terminal,false,undefined,{wheelScrollLines:3});ui.setLayoutRoot(outer);ui.start();ui.renderNow();
 const positions=[];
 for(const data of ['\x1b[<65;2;2M','\x1b[<65;2;5M','\x1b[H','\x1b[<0;12;2M','\x1b[<32;12;1M','\x1b[<0;12;1m']) {onInput(data);ui.renderNow();positions.push([inner.scrollTop,outer.scrollTop]);}
 mouse.push({overscroll,positions});ui.stop({preserveScreen:true});
}
const trackDrag={};
for(const scrollbar of ['always','auto']) {
 let input;const terminal={columns:12,rows:5,start(callback){input=callback},stop(){},write(){},hideCursor(){},showCursor(){},kittyProtocolActive:false};
 const scroll=new ScrollView({render:()=>Array.from({length:20},(_,i)=>`row ${i}`),invalidate(){}},{scrollbar});
 const ui=new TuiAltScreen(terminal);ui.setLayoutRoot(scroll);ui.start();ui.renderNow();const positions=[];
 for(const key of ['\x1b[<0;12;4M','\x1b[<32;12;5M']){input(key);ui.renderNow();positions.push(scroll.scrollTop)}
 trackDrag[scrollbar]=positions;ui.stop({preserveScreen:true});
}
const wide=new ScrollView({render:()=>['中文','中文','中文','中文'],invalidate(){}},{scrollbar:'always'});
const wideLines=renderLayoutFrame(wide,3,2,()=>{}).lines.map(l=>stripTerminalSequences(l).trimEnd());
const input={allocation,scenario:'follow, history, append, resize, end, shrink, tiny viewport; nested hit; horizontal alignment',viewportSteps:[[12,7],[12,7],[12,7],[8,6],[8,6],[8,6],[8,3]],initialRows:12,appendRows:2,scrollDelta:-3,mouse:{overscroll:['chain','contain'],wheelLines:3,keys:['\x1b[<65;2;2M','\x1b[<65;2;5M','\x1b[H','\x1b[<0;12;2M','\x1b[<32;12;1M','\x1b[<0;12;1m']},trackDrag:{modes:['always','auto'],rows:20,height:5,keys:['\x1b[<0;12;4M','\x1b[<32;12;5M']}};
const outcome={states,mouse,trackDrag,wideLines,allocation:allocation.map(c=>allocateStackSizes(c.entries,c.intrinsic,c.available,c.gap)),hits:[[1,1],[1,4],[12,0]].map(([x,y])=>getScrollViewsAt(nested,x,y).map(s=>s===inner?'inner':'outer')),horizontal:horizontal.lines.map(l=>stripTerminalSequences(l).trimEnd())};
const c={schema_version:'1.0.0',id:'tui/layout-scrolling',catalog_id:'contract:tui/layout-scrolling',surface:'go-sdk',input,observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/tui/src/layout.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/layout-scrolling.mjs <locked-pi-checkout>',platform:'any',environment:{colors:'stripped'}};
const path='parity/oracle/fixtures/layout-scrolling.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(JSON.stringify(outcome,null,2));
