import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';
import {caseDigest,observationDigest} from './fixture-hash.mjs';
const pi=resolve(process.argv[2]),lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
delete process.env.NO_COLOR; process.env.COLORTERM='truecolor'; process.env.FORCE_COLOR='3';
const api=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/theme/theme.js`));
const terminal=await import(pathToFileURL(`${pi}/packages/tui/dist/terminal-colors.js`));
const {ThemeSelectorComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/theme-selector.js`));
api.initTheme('dark',false);
const reports=['\x1b]11;rgb:ffff/ffff/ffff\x07','\x1b]11;#000000\x1b\\','\x1b]11;rgba:80/40/ff/ff\x07','\x1b]11;bad\x07','\x1b[?997;1n','\x1b[?997;2n','\x1b[?997;1n\x1b[?997;2n','x'];
const env=['','0;15','15;0','0;7','0;invalid;255'];
const keys=[['\r'],['\x1b[B','\r'],['\x1b[A','\x1b'],['\x1b[B','\x1b[A','\r']];
const selectors=keys.map(keys=>{const events=[];const s=new ThemeSelectorComponent('dark',v=>events.push('select:'+v),()=>events.push('cancel'),v=>events.push('preview:'+v));for(const k of keys)s.getSelectList().handleInput(k);return events});
const syntax = [
 ['go', 'package main\n// note\nfunc main() { println("hi", 42) }'],
 ['js', 'const x = /ab+/g; // note\nfunction f(a) { return true; }'],
 ['typescript', 'interface User { name: string }\nconst n: number = 1;'],
 ['python', 'def greet(name):\n    # note\n    return "hi" + name'],
 ['json', '{"name": true, "count": 12, "list": [null]}'],
 ['bash', 'echo "$HOME" # note'], ['html', '<div class="x">&amp;</div>'],
 ['css', 'body { color: #ffffff; }'], ['rust', 'fn main() { let x = 42; }'],
 ['java', 'public class Hello { int x = 12; }'], ['c', '#include <stdio.h>\nint main() { return 0; }'],
 ['cpp', 'std::string s = "hello";'], ['sql', "SELECT * FROM users WHERE name = 'hello';"],
 ['yaml', 'name: hello\nitems: [true, 42]'], ['diff', '+added\n-removed'],
 ['markdown', '# Header\n**bold** *italic* [link](url)'],
 ['ruby', 'def hello(name)\n  puts "hi #{name}"\nend'],
 ['swift', 'let name: String = "hello"'], ['kotlin', 'fun main() { println("hello") }'],
 ['', 'this is plain code\nand more'], ['unknown', 'if true then false'],
 ['go', '/* multiline\ncomment */\nvar s = `multi\nline`'],
];
const highlights = {};
for (const name of ['dark', 'light']) {
 api.initTheme(name, false);
 highlights[name] = syntax.map(([lang, code]) => api.highlightCode(code, lang));
}
const {SettingsSelectorComponent}=await import(pathToFileURL(`${pi}/packages/coding-agent/dist/modes/interactive/components/settings-selector.js`));
const menuKeys=[['\x1b[A','\x1b'],['\x1b[B','\r'],['\x1b[A','\r','\r','\x1b[B','\r','\x1b[B','\x1b[B','\r'],['\x1b[A','\r','\x1b[B','\r','\x1b[B','\x1b','\x1b'],['\x1b[A','\r','\x1b[B','\x1b[B','\x1b[B','\r','\x1b']];
api.initTheme('dark',false);
const menus=menuKeys.map(keys=>{
 const events=[];
 const callbacks=new Proxy({onThemePreview:v=>events.push('preview:'+v),onThemeChange:v=>events.push('select:'+v)}, {get:(obj,k)=>obj[k]??(()=>{})});
 const config={currentTheme:'dark',terminalTheme:'dark',availableThemes:['dark','light'],availableThinkingLevels:['off'],thinkingLevel:'off',warnings:{},httpIdleTimeoutMs:300000,autocompleteMaxVisible:10};
 const s=new SettingsSelectorComponent(config,callbacks), l=s.getSettingsList();
 l.handleInput('theme'); l.handleInput('\r');
 const screens=[s.render(80).slice(1,-1).map(x=>x.replace(/\x1b\[[0-9;]*m/g,'').trimEnd())];
 for(const key of keys){ l.handleInput(key); screens.push(s.render(80).slice(1,-1).map(x=>x.replace(/\x1b\[[0-9;]*m/g,'').trimEnd())); }
 // After completion the parent settings list is verified by the shared CLI harness.
 return {events,screens:screens.slice(0,-1)};
});
const {Markdown}=await import(pathToFileURL(`${pi}/packages/tui/dist/components/markdown.js`));
const markdown = [
 '# Head `code` tail **bold** end',
 '## Head *italic* ~~gone~~ tail',
 'Plain **bold *nested* tail** and `code` after [label](https://example.com) end.',
 '> Quote **bold** `code` tail\n>\n> - item *italic* tail\n> - second',
 '- item **bold** `code` tail\n    - nested ~~strike~~ after\n\n1. first\n2. second',
 '```go\nfunc main() { println("hi", 42) }\n```\n\nAfter code.',
 '---\n\nEmail <a@example.com> and <https://example.com> tail.',
];
const markdownWidths=markdown.map(()=>80);
for(const source of [...markdown]){markdown.push(source);markdownWidths.push(24);}
const markdownResults={};
for(const name of ['dark','light']) {
 api.initTheme(name,false);
 markdownResults[name]=markdown.map((text,index)=>new Markdown(text,0,0,api.getMarkdownTheme(),{color:s=>api.theme.fg('text',s),italic:true}).render(markdownWidths[index]));
}
const {SettingsList}=await import(pathToFileURL(`${pi}/packages/tui/dist/components/settings-list.js`));
const settingsKeys=['theme','\x1b[B',' ','\r','\x15','absent','\x1b'];
const settingsItems=[{id:'theme',label:'Theme',description:'Color theme for the interface',currentValue:'dark',values:['dark','light']},{id:'light',label:'Light theme',description:'Theme for light appearance',currentValue:'light',values:['dark','light']}];
const settingsEvents=[];
const settingsList=new SettingsList(settingsItems,10,api.getSettingsListTheme(),(id,value)=>settingsEvents.push(`${id}:${value}`),()=>settingsEvents.push('cancel'),{enableSearch:true});
const settingsScreens=[settingsList.render(80).map(x=>x.replace(/\x1b\[[0-9;]*m/g,'').trimEnd())];
for(const key of settingsKeys){settingsList.handleInput(key);settingsScreens.push(settingsList.render(80).map(x=>x.replace(/\x1b\[[0-9;]*m/g,'').trimEnd()));}
const outcome={settings:{events:settingsEvents,screens:settingsScreens},markdown:markdownResults,menus,syntax:highlights,reports:reports.map(s=>({rgb:terminal.parseOsc11BackgroundColor(s)??null,scheme:terminal.parseTerminalColorSchemeReport(s)??''})),env:env.map(COLORFGBG=>api.detectTerminalBackgroundFromEnv({env:{COLORFGBG}}).theme),selectors};
const c={schema_version:'1.0.0',id:'sdk/interactive/theme-runtime',catalog_id:'contract:codingagent/interactive-themes',surface:'go-sdk',input:{reports,env,keys,syntax,menuKeys,markdown,markdownWidths,settingsKeys},observe:['outcome','side_effects']};
const observation={outcome,side_effects:[]};
const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:'packages/coding-agent/src/modes/interactive/theme/theme-controller.ts'},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node parity/oracle/theme-runtime.mjs <locked-pi-checkout>',platform:'any',environment:{}};
const path='parity/oracle/fixtures/theme-runtime.json';
if(process.argv.includes('--check')){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(path))))throw Error('fixture drift');}else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.log(outcome);
