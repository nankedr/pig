ObjC.import('AppKit');
ObjC.import('Foundation');
function run(argv) {
 const pb=$.NSPasteboard.generalPasteboard,path=$(argv[1]);
 if(argv[0]==='save') {
  const snapshot=[];
  const items=pb.pasteboardItems;
  for(let i=0;i<items.count;i++){
   const item=items.objectAtIndex(i),types=item.types,row={};
   for(let j=0;j<types.count;j++){
    const type=types.objectAtIndex(j),data=item.dataForType(type);
    if(data)row[ObjC.unwrap(type)]=ObjC.unwrap(data.base64EncodedStringWithOptions(0));
   }
   snapshot.push(row);
  }
  $(JSON.stringify(snapshot)).writeToFileAtomicallyEncodingError(path,true,$.NSUTF8StringEncoding,null);
 }else if(argv[0]==='fixture'){
  const data=$.NSData.dataWithContentsOfFile(path);
  pb.clearContents;pb.setDataForType(data,$(argv[2]||'public.png'));
 }else if(argv[0]==='text'){pb.clearContents;pb.setStringForType($(argv[1]),$('public.utf8-plain-text'));
 }else{
  const snapshot=JSON.parse(ObjC.unwrap($.NSString.stringWithContentsOfFileEncodingError(path,$.NSUTF8StringEncoding,null))),items=$.NSMutableArray.new;
  for(const row of snapshot){const item=$.NSPasteboardItem.new;for(const [type,encoded] of Object.entries(row)){const data=$.NSData.alloc.initWithBase64EncodedStringOptions($(encoded),0);item.setDataForType(data,$(type));}items.addObject(item);}
  pb.clearContents;if(items.count)pb.writeObjects(items);
 }
 return 'OK';
}
