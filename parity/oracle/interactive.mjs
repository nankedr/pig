import { execFileSync } from 'node:child_process';
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { caseDigest, observationDigest } from './fixture-hash.mjs';
const pi = resolve(process.argv[2]);
const lock = JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if (execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim()) throw Error('dirty baseline');
const outcome = JSON.parse(execFileSync('python3',['parity/terminal/oracle.py',pi],{encoding:'utf8'}));
outcome.exits = [];
for(const action of ['ctrl-c-twice','quit','SIGTERM','SIGHUP']) {
 const observed=JSON.parse(execFileSync('python3',['parity/terminal/oracle.py',pi,action],{encoding:'utf8'}));
 outcome.exits.push({action,exit_code:observed.exit_code,terminal_restored:observed.terminal_restored,cursor_restored:observed.cursor_restored,paste_disabled:observed.paste_disabled});
}
const sigint=JSON.parse(execFileSync('python3',['parity/terminal/oracle.py',pi,'SIGINT'],{encoding:'utf8'}));
outcome.sigint={exit_code:sigint.exit_code,terminal_restored:sigint.terminal_restored,cursor_restored:sigint.cursor_restored,paste_disabled:sigint.paste_disabled};
const c = {schema_version:'1.0.0',id:'cli/interactive/text-conversation',catalog_id:'contract:codingagent/interactive-text',surface:'cli',input:{prompts:['first question','second question'],responses:['FIRST_STREAM_TOKEN FIRST_DONE','SECOND_DONE'],exits:['ctrl-d','ctrl-c-twice','quit','SIGTERM','SIGHUP'],raw_signal:'SIGINT',terminal:{rows:32,columns:100}},observe:['outcome','side_effects']};
const observation = {outcome,side_effects:[]};
const fixture = {schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/interactive-mode.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/interactive.mjs <locked-pi-checkout>',platform:'posix',environment:{terminal:'xterm-256color',transport:'real child process + PTY + gated loopback SSE'}};
const path='parity/oracle/fixtures/interactive.json';
if(process.argv.includes('--check')) {if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path)))) throw Error('fixture drift');} else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);
