import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync,mkdtempSync,rmSync} from 'node:fs';
import {resolve,join} from 'node:path';
import {tmpdir} from 'node:os';
const pi=resolve(process.argv[2]);
const lock=JSON.parse(readFileSync('parity/baseline/upstream.lock.json'));
if(execFileSync('git',['-C',pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw Error('baseline mismatch');
if(execFileSync('git',['-C',pi,'status','--porcelain','--untracked-files=no'],{encoding:'utf8'}).trim())throw Error('dirty baseline');
if(JSON.parse(readFileSync(join(pi,'node_modules/highlight.js/package.json'))).version!=='10.7.3')throw Error('highlight.js version mismatch');
const dir=mkdtempSync(join(tmpdir(),'pig-highlight-'));
try {
 const entry=join(dir,'entry.js'),output=join(dir,'highlight.js');
 writeFileSync(entry,`import hljs from ${JSON.stringify(join(pi,'node_modules/highlight.js/lib/index.js'))};\nglobalThis.pigHighlight = (code, language) => hljs.getLanguage(language) ? hljs.highlight(code, {language, ignoreIllegals:true}).value : null;\n`);
 execFileSync(join(pi,'node_modules/esbuild/bin/esbuild'),[entry,'--bundle','--format=iife','--minify',`--outfile=${output}`]);
 const path='codingagent/syntax/highlight.js',data=readFileSync(output);
 if(process.argv.includes('--check')){if(!data.equals(readFileSync(path)))throw Error('highlight bundle drift');}
 else writeFileSync(path,data);
} finally {rmSync(dir,{recursive:true,force:true});}
