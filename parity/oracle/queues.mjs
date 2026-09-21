import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { caseDigest, observationDigest } from './fixture-hash.mjs';
const pi = resolve(process.argv[2]);
const lock = JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if (execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim()) throw Error('dirty baseline');
for (const scenario of ['queue','restore','abort','retry','retry-success','commands']) {
const outcome = JSON.parse(execFileSync('python3',['parity/terminal/queues.py',pi,scenario],{encoding:'utf8'}));
const c = {schema_version:'1.0.0',id:'cli/interactive/queues/'+scenario,catalog_id:'contract:codingagent/interactive-queues',surface:'cli',input:{scenario,steps:scenario==='commands'?['submit first question','await partial','alt+enter /quit','alt+enter /new','release','submit after question']:scenario.startsWith('retry')?['submit first question','await retry',scenario==='retry'?'escape':'await retry success','submit after question']:['submit first question','await partial','alt+enter follow question','enter steer question',scenario==='queue'?'release then submit after question':scenario==='restore'?'alt+up with draft; ctrl+c; release; submit after question':'escape with draft; append edited; submit'],terminal:{rows:32,columns:100}},observe:['outcome','side_effects']};
const observation = {outcome,side_effects:[]};
const fixture = {schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/interactive-mode.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/queues.mjs <locked-pi-checkout>',platform:'posix',environment:{terminal:'xterm-256color',transport:'real child process + PTY + gated loopback SSE'}};
const path='parity/oracle/fixtures/queues'+(scenario==='queue'?'':'-'+scenario)+'.json';
if(process.argv.includes('--check')) {if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path)))) throw Error('fixture drift');} else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);

}
