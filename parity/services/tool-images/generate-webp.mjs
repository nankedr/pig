import {readFileSync,writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';
import {execFileSync} from 'node:child_process';
const checkout=process.argv[2];
if(execFileSync('git',['-C',checkout,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!=='936aff00918de1187f085f123c2812d8f2d67745') throw Error('unlocked Pi');
const {default:photon}=await import(pathToFileURL(join(checkout,'node_modules/@silvia-odwyer/photon-node/photon_rs.js')));
// wide.png is Pig's generated red/blue color-block fixture; Photon encodes it as WebP.
const img=photon.PhotonImage.new_from_byteslice(readFileSync(process.argv[3]));
const width=img.get_width(),height=img.get_height(),webp=Buffer.from(img.get_bytes_webp());img.free();
const chunk=(id,data)=>{const h=Buffer.alloc(8);h.write(id);h.writeUInt32LE(data.length,4);return Buffer.concat([h,data,Buffer.alloc(data.length%2)]);};
const ext=Buffer.alloc(10);ext[0]=8;ext.writeUIntLE(width-1,4,3);ext.writeUIntLE(height-1,7,3);
for(let orient=1;orient<=8;orient++) {
const exif=Buffer.from([73,73,42,0,8,0,0,0,1,0,0x12,1,3,0,1,0,0,0,orient,0,0,0,0,0,0,0]);
const content=Buffer.concat([chunk('VP8X',ext),webp.subarray(12),chunk('EXIF',exif)]);
const head=Buffer.alloc(12);head.write('RIFF');head.writeUInt32LE(content.length+4,4);head.write('WEBP',8);
writeFileSync(`${process.argv[4]}/orientation-${orient}.webp`,Buffer.concat([head,content]));
}
