import assert from 'node:assert/strict';
import {execFileSync,spawnSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdtempSync,rmSync} from 'node:fs';
import {join,resolve} from 'node:path';
import {tmpdir} from 'node:os';
import {pathToFileURL} from 'node:url';
import {openBrowser} from './browser.mjs';
const root=resolve(import.meta.dirname,'../..'),dir=mkdtempSync(join(tmpdir(),'pig-image-export-'));
const binary=process.env.PIG_BINARY||join(dir,'pig');
try{
 if(!process.env.PIG_BINARY)execFileSync('go',['build','-o',binary,'./cmd/pig'],{cwd:root});
 const fixture=JSON.parse(readFileSync(join(root,'parity/oracle/fixtures/export-html.json')));
 const entries=fixture.case.input.session.trim().split('\n').map(JSON.parse);
 const image={type:'image',mimeType:'image/png',data:readFileSync(join(root,'parity/services/user-image.png')).toString('base64')};
 const user=entries.find(e=>e.message?.role==='user'),tool=entries.find(e=>e.message?.role==='toolResult');
 user.message.content=[{type:'text',text:'![remote](https://example.com/track.png) <img src=x onerror="window.__xss=1">'},image];
 tool.message.content.push(image);
 const source=join(dir,'session.jsonl'),output=join(dir,'session.html');
 const save=()=>writeFileSync(source,entries.map(JSON.stringify).join('\n')+'\n');
 const run=()=>execFileSync(binary,['--export',source,output],{cwd:dir,env:{...process.env,PIG_OFFLINE:'1',PIG_CODING_AGENT_DIR:join(dir,'state')}});
 save();run();rmSync(source);
 const browser=await openBrowser();
 try{
  const page=await browser.newPage(),requests=[],errors=[];
  page.on('request',r=>{if(/^https?:|^file:/.test(r.url())&&r.url()!==pathToFileURL(output).href)requests.push(r.url())});
  page.on('pageerror',e=>errors.push(e.message));
  await page.goto(pathToFileURL(output).href);await page.locator('#messages .message-image').waitFor();
  const observation=await page.evaluate(()=>({images:[...document.querySelectorAll('#messages img')].map(i=>({loaded:i.complete&&i.naturalWidth===1&&i.naturalHeight===1,source:i.src.startsWith('data:image/png;base64,')})),executed:!!window.__xss,deferred:!!document.querySelector('.deferred-image')}));
  assert.deepEqual(observation,{images:[{loaded:true,source:true},{loaded:true,source:true}],executed:false,deferred:true});
  await page.locator('.message-image').click();await page.waitForFunction(()=>document.getElementById('modal-image').naturalWidth===1);
  assert.deepEqual(requests,[]);assert.deepEqual(errors,[]);
  console.log('PASS offline inline user/tool images, modal, no file/network requests and XSS');
 }finally{await browser.close()}
 for(const bad of [{...image,mimeType:'image/svg+xml'}, {...image,data:'file:///etc/passwd'}, {...image,mimeType:'image/png" onerror="alert(1)'}, {...image,data:'aGk='}]){
  user.message.content=[bad];save();const before=readFileSync(output);
  const result=spawnSync(binary,['--export',source,output],{cwd:dir,env:{...process.env,PIG_OFFLINE:'1',PIG_CODING_AGENT_DIR:join(dir,'state')}});
  assert.notEqual(result.status,0);assert.deepEqual(readFileSync(output),before);
 }
 console.log('PASS malformed/SVG/URL/attribute payloads rejected before output mutation');
}finally{rmSync(dir,{recursive:true,force:true})}
