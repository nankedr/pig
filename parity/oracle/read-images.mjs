import {readFileSync,writeFileSync} from 'node:fs';
import {execFileSync} from 'node:child_process';
import {dirname,join} from 'node:path';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {createHash} from 'node:crypto';
const root=join(dirname(fileURLToPath(import.meta.url)),'../..');
const checkout=process.argv.slice(2).find(a=>!a.startsWith('--'));
const commit=execFileSync('git',['-C',checkout,'rev-parse','HEAD'],{encoding:'utf8'}).trim();
if(commit!==JSON.parse(readFileSync(join(root,'parity/baseline/upstream.lock.json'))).upstream.commit) throw Error('unlocked Pi');
const {createReadTool}=await import(pathToFileURL(join(checkout,'packages/coding-agent/dist/core/tools/read.js')));
const photon=await import(pathToFileURL(join(checkout,'node_modules/@silvia-odwyer/photon-node/photon_rs.js')));
const files=['../user-image.png','wide.png','canvas.gif','tiny.bmp','corrupt.png',...Array.from({length:8},(_,i)=>`orientation-${i+1}.jpg`),...Array.from({length:8},(_,i)=>`orientation-${i+1}.webp`)];
const cases=[];
for(const file of files) for(const autoResize of [true,false]) {
 const path=join(root,'parity/services/tool-images',file);
 const tool=createReadTool(root,{autoResizeImages:autoResize});
 const result=await tool.execute('read-fixture',{path,offset:999,limit:0});
 const content=result.content.map(b=> {
  if(b.type==='text') return b;
  const bytes=Buffer.from(b.data,'base64');let img;try { img=photon.PhotonImage.new_from_byteslice(bytes); } catch { return {...b,undecodable:true}; }
  const width=img.get_width(),height=img.get_height(),raw=img.get_raw_pixels();
  const corners=[[0,0],[width-1,0],[0,height-1],[width-1,height-1]].map(([x,y])=>{const p=(y*width+x)*4;return raw[p]>raw[p+2]?'red':raw[p+2]>raw[p]?'blue':'other'});
  img.free();
  const preserve=bytes.equals(readFileSync(path));
  return {type:'image',mimeType:b.mimeType,width,height,corners,...(preserve?{data:b.data}:{})};
 });
 cases.push({file,autoResize,content});
}
const fixture={schema_version:1,baseline_commit:commit,scope:'public Pi read; semantic dimensions/orientation/corner colors and notes; unchanged images exact bytes; resized/converted encoder bytes excluded (Go CatmullRom/PNG/JPEG vs Photon Lanczos3)',cases,observation_hash:createHash('sha256').update(JSON.stringify(cases)).digest('hex')};
const path=join(root,'parity/oracle/fixtures/read-images.json');
if(process.argv.includes('--check')) {if(JSON.stringify(JSON.parse(readFileSync(path)))!==JSON.stringify(fixture)) throw Error('read image Oracle drift')}
else writeFileSync(path,JSON.stringify(fixture,null,2)+'\n');
console.error('fixed Pi read image semantic fixture reproduces');
