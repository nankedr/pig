import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync } from 'node:fs';
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
 const themes=JSON.parse(readFileSync(join(root,'parity/oracle/fixtures/themes.json')));
 const themePath=join(dir,'theme.json');mkdirSync(join(dir,'state'),{recursive:true});
 const themedBrowser=await openBrowser();
 try {
  const page=await themedBrowser.newPage();const requests=[];page.on('request',r=>{if(/^https?:/.test(r.url()))requests.push(r.url())});
  for(let i=0;i<themes.case.input.documents.length;i++){
   const doc=themes.case.input.documents[i],expected=themes.observation.outcome.colors[i];
   writeFileSync(themePath,JSON.stringify(doc));writeFileSync(join(dir,'state/settings.json'),JSON.stringify({theme:doc.name}));
   execFileSync(binary,['--export',source,html,'--theme',themePath,'--no-themes'],{cwd:dir,env:{...process.env,PIG_OFFLINE:'1',PIG_CODING_AGENT_DIR:join(dir,'state')}});
   await page.goto(pathToFileURL(html).href);await page.locator('#messages .user-message').first().waitFor();
   const colors=await page.evaluate(keys=>Object.fromEntries(keys.map(key=>[key,getComputedStyle(document.documentElement).getPropertyValue('--'+key).trim()])),Object.keys(expected.html));
   assert.deepEqual(colors,expected.html);
  }
  const bad=structuredClone(themes.case.input.documents[2]);bad.name='custom</style><script>window.__themeXss=1</script>';bad.export.pageBg='#fff;</style><script>window.__themeXss=1</script>';
  writeFileSync(themePath,JSON.stringify(bad));writeFileSync(join(dir,'state/settings.json'),'{}');
  execFileSync(binary,['--export',source,html,'--theme',themePath],{cwd:dir,env:{...process.env,PIG_OFFLINE:'1',PIG_CODING_AGENT_DIR:join(dir,'state')},stdio:['ignore','pipe','pipe']});
  await page.goto(pathToFileURL(html).href);assert.equal(await page.evaluate(()=>!!window.__themeXss),false);assert.deepEqual(requests,[]);
  console.log('PASS: dark/light/custom theme CSS matches locked Pi HTML; theme injection rejected');
 }finally{await themedBrowser.close();rmSync(join(dir,'state/settings.json'));}

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
 entries.push({type:'message',id:'bash-text',parentId:badID,timestamp:last.timestamp,message:{role:'bashExecution',command:attack,output:attack,exitCode:1,cancelled:false,truncated:false}});
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
