import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
for (const mode of ['regular','fullscreen']) {
const outcome=JSON.parse(execFileSync('python3',['parity/terminal/m6-workflow.py',pi,...(mode==='fullscreen'?['--fullscreen']:[])],{encoding:'utf8'}));
const c={schema_version:'1.0.0',id:'cli/interactive/m6-workflow/'+mode,catalog_id:'contract:codingagent/m6-workflow',surface:'cli',input:{mode,steps:['theme watch retains draft','settings and selector cancellation','excluded Bash','paste and external editor','session stats','compact with instructions and continued context','reload templates skills and context','HTML export and failure','compaction failure and cancellation','continue and exit','steering and follow-up queue after maintenance','resize and page navigation'],terminal:{rows:40,columns:120}},observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/interactive-mode.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/m6-workflow.mjs <locked-pi-checkout>',platform:'posix',environment:{terminal:'xterm-256color',transport:'real child process + PTY; offline controlled SSE and local resources and HTML'}};
const path='parity/oracle/fixtures/m6-workflow-'+mode+'.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);
}
