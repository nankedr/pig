import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { openBrowser, observeExport } from './browser.mjs';

const root=resolve(import.meta.dirname,'../..');
const dir=mkdtempSync(join(tmpdir(),'pig-html-browser-'));
const binary=process.env.PIG_BINARY || join(dir,'pig');
try {
 if(!process.env.PIG_BINARY) execFileSync('go',['build','-o',binary,'./cmd/pig'],{cwd:root});
 const source=join(dir,'session.jsonl'), html=join(dir,'session.html');
 const fixture=JSON.parse(readFileSync(join(root,'parity/oracle/fixtures/export-html.json')));
 writeFileSync(source,fixture.case.input.session);
 const run=()=>execFileSync(binary,['--export',source,html],{cwd:dir,env:{...process.env,PIG_OFFLINE:'1',PIG_CODING_AGENT_DIR:join(dir,'state')}});
 run();
 assert.deepEqual(await observeExport(html),fixture.observation.outcome.rendered);
 const entries=fixture.case.input.session.trim().split('\n').map(JSON.parse);
 const attack='<img src=x onerror="window.__xss=1"><svg onload="window.__xss=1"></svg></script><script>window.__xss=1</script>';
 const markdown=[attack,'[bad](javascript:window.__xss=1)','[bad](JaVaScRiPt:window.__xss=1)','[bad](java\u0001script:window.__xss=1)','[bad](data:text/html,test)','[bad](vbscript:msgbox)','[attr](https://example.com/\"onmouseover=\"window.__xss=1)','![image](https://example.com/track.png)','`'+attack+'`','```html\n'+attack+'\n```','[safe](https://example.com)'].join('\n\n');
 entries[0].id=attack;
 for(const e of entries) {
  if(e.type==='branch_summary')e.summary=markdown;
  if(e.type==='compaction')e.summary=attack;
  if(e.type==='label')e.label=attack;
  if(e.message?.role==='user')e.message.content=markdown;
  if(e.message?.role==='assistant') {
   e.message.model=attack;e.message.provider=attack;
   e.message.content.push({type:'text',text:markdown});
   const call=e.message.content.find(x=>x.type==='toolCall');
   call.arguments.offset=attack;
  }
  if(e.message?.role==='toolResult')e.message.content=[{type:'text',text:attack}];
 }
 const last=entries.at(-1);const badID='id" onmouseover="window.__xss=1';
 entries.push({type:'message',id:badID,parentId:last.id,timestamp:last.timestamp,message:{role:'user',content:markdown,timestamp:1}});
 writeFileSync(source,entries.map(JSON.stringify).join('\n')+'\n');run();
 const browser=await openBrowser();
 try {
  const page=await browser.newPage();const errors=[],requests=[];
  page.on('pageerror',e=>errors.push(e.message));
  page.on('request',r=>{if(/^https?:/.test(r.url()))requests.push(r.url());});
  await page.goto(pathToFileURL(html).href);
  await page.locator('#messages .user-message').first().waitFor();
  await page.locator('[data-filter="all"]').click();
  await page.locator('[data-action="toggle-thinking"]').click();
  await page.locator('[data-action="toggle-tools"]').click();
  const expanded=await page.locator('.compaction').first().evaluate(x=>x.classList.contains('expanded'));
  await page.locator('.compaction').first().click();
  assert.notEqual(await page.locator('.compaction').first().evaluate(x=>x.classList.contains('expanded')),expanded);
  const safety=await page.evaluate(()=>({
   executed:!!window.__xss,
   images:document.querySelectorAll('#messages img, #messages svg[onload]').length,
   unsafeLinks:[...document.querySelectorAll('#messages a[href]')].filter(x=>/^(javascript|vbscript|data):/i.test(x.getAttribute('href'))).length,
   injectedHandlers:[...document.querySelectorAll('*')].some(x=>[...x.attributes].some(a=>/^on/i.test(a.name))),
   literal:document.getElementById('messages').textContent.includes('</script>'),
   deferred:document.querySelectorAll('.deferred-image').length>0,
   safeLinks:document.querySelectorAll('a[href="https://example.com"]').length>0,
  }));
  assert.deepEqual(safety,{executed:false,images:0,unsafeLinks:0,injectedHandlers:false,literal:true,deferred:true,safeLinks:true});
  assert.deepEqual(requests,[]);assert.deepEqual(errors,[]);
  if(process.env.PIG_HTML_SCREENSHOT)await page.screenshot({path:process.env.PIG_HTML_SCREENSHOT,fullPage:false});
  console.log('PASS: fixed Pi rendering, branch navigation, offline assets and browser XSS attacks');
 } finally {await browser.close();}
} finally {rmSync(dir,{recursive:true,force:true});}
