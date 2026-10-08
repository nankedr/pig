import { readFileSync, writeFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { dirname, join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { createHash } from 'node:crypto';
const root=join(dirname(fileURLToPath(import.meta.url)), '../..');
const checkout=process.argv.slice(2).find(a=>!a.startsWith('--')) ?? join(root,'.upstream/pi');
const lock=JSON.parse(readFileSync(join(root,'parity/baseline/upstream.lock.json')));
const commit=execFileSync('git',['-C',checkout,'rev-parse','HEAD'],{encoding:'utf8'}).trim();
if(commit!==lock.upstream.commit) throw Error('Pi checkout differs from fixed baseline');
const {convertResponsesMessages}=await import(pathToFileURL(join(checkout,'packages/ai/src/api/openai-responses-shared.ts')));
const {convertMessages}=await import(pathToFileURL(join(checkout,'packages/ai/src/api/openai-completions.ts')));
const image={type:'image',mimeType:'image/png',data:readFileSync(join(root,'parity/services/user-image.png')).toString('base64')};
const text={type:'text',text:'before'};
const cases=[];
for(const api of ['openai-responses','openai-completions']) {
 const model={id:'vision-fixture',name:'Fixture',api,provider:'deepseek',baseUrl:'https://local.invalid',input:['text','image'],reasoning:false,cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:512};
 for(const content of [[text,image,{type:'text',text:'after'}],[image]]) {
  const context={messages:[{role:'user',content,timestamp:1}]};
  const wire=api==='openai-responses'?convertResponsesMessages(model,context,new Set()):convertMessages(model,context,{});
  cases.push({model,context,wire});
 }
}
const fixture={schema_version:1,baseline_commit:commit,scope:'valid inline user images in vision models; no service capability, processing or unsupported-model downgrade claims',cases};
fixture.observation_hash=createHash('sha256').update(JSON.stringify(cases)).digest('hex');
const path=join(root,'parity/oracle/fixtures/user-images.json');
if(process.argv.includes('--check')) { if(JSON.stringify(JSON.parse(readFileSync(path)))!==JSON.stringify(fixture)) throw Error('user image Oracle drift'); }
else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.error('fixed Pi user image wire fixture reproduces');
