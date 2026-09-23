import fcntl, json, os, pathlib, pty, select, struct, subprocess, sys, tempfile, termios, time
binary=pathlib.Path(sys.argv[1]).resolve();pig='--pig' in sys.argv
root_repo=pathlib.Path(__file__).resolve().parents[2]
results=[]
for mode in ['truecolor','256color','auto']:
 with tempfile.TemporaryDirectory(prefix='theme120-') as tmp:
  root=pathlib.Path(tmp);agent=root/'agent';agent.mkdir();(agent/'themes').mkdir()
  theme=json.loads((root_repo/'codingagent/themes/dark.json').read_text());theme['name']='local'
  for k in theme['colors']:theme['colors'][k]='#112233'
  path=agent/'themes/local.json';path.write_text(json.dumps(theme))
  settings=agent/'settings.json';settings.write_text(json.dumps({'theme':'light/local' if mode=='auto' else 'local','quietStartup':True,'compaction':{'enabled':False}}))
  source=root/'session.jsonl';timestamp='2026-01-01T00:00:00.000Z'
  entries=[{'type':'session','version':3,'id':'theme120','timestamp':timestamp,'cwd':str(root)}, {'type':'message','id':'a','parentId':None,'timestamp':timestamp,'message':{'role':'assistant','content':[{'type':'text','text':'# THEME_HISTORY'}],'api':'openai-completions','provider':'deepseek','model':'deepseek-v4-flash','usage':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'totalTokens':0,'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'total':0}},'stopReason':'stop','timestamp':1767225600000}}]
  source.write_text(''.join(json.dumps(x)+'\n' for x in entries));original=source.read_bytes()
  master,slave=pty.openpty();before=termios.tcgetattr(slave);fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0))
  env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','COLORTERM':'truecolor' if mode!='256color' else '', 'PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
  if pig:env.update({'PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1'})
  args=([str(binary)] if pig else ['node',str(binary/'packages/coding-agent/dist/cli.js')])+['--session',str(source),'--no-tools','--no-extensions','--no-skills','--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic']
  p=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root);output=b''
  def pump(seconds=.3):
   global output
   end=time.monotonic()+seconds
   while time.monotonic()<end:
    if select.select([master],[],[],.03)[0]:output+=os.read(master,65536)
  def wait(needle):
   end=time.monotonic()+12
   while needle not in output:
    if time.monotonic()>end or p.poll()!=None:raise RuntimeError(f'missing {needle!r}: {output[-6000:]!r}')
    pump(.05)
  def send(data):
   global output
   output=b'';os.write(master,data);pump()
  def menu():
   send(b'/settings\r')
   send(b'theme')
   send(b'\r')
  try:
   if mode=='auto':
    wait(b'\x1b[?996n');os.write(master,b'\x1b]11;rgb:ffff/ffff/ffff\x07\x1b[?997;1n')
   wait(b'THEME_HISTORY');pump(.5)
   old=b'\x1b[38;2;17;34;51m' if mode!='256color' else b'\x1b[38;5;17m'
   wait(old)
   send(b'DRAFT_RETAINED')
   output=b''
   for k in theme['colors']:theme['colors'][k]='#445566'
   temporary=path.with_suffix('.tmp');temporary.write_text(json.dumps(theme));temporary.replace(path)
   new=b'\x1b[38;2;68;85;102m' if mode!='256color' else b'\x1b[38;5;59m'
   wait(new);wait(b'DRAFT_RETAINED')
   path.write_text('{bad');pump(.3);path.unlink();pump(.3)
   path.write_text(json.dumps(theme));pump(.3)
   if mode=='auto':
    for _ in range(3):
     send(b'\x1b[?997;2n');wait(b'\x1b[38;2;154;115;38m')
     send(b'\x1b[?997;1n');wait(new)
    send(b'\x03');cancelled=True;saved=json.loads(settings.read_text())['theme']
   else:
    send(b'\x03');menu();send(b'\x1b[A');send(b'\x1b[27u');wait(b'Type to search');send(b'\x1b[27u')
    cancelled=json.loads(settings.read_text())['theme']=='local'
    menu();send(b'\x1b[A');send(b'\r');pump(.3)
    saved=json.loads(settings.read_text())['theme']
    wait(b'Type to search')
   send(b'\x1b[27u');send(b'\x04')
   end=time.monotonic()+8
   while p.poll() is None:
    if time.monotonic()>end:raise RuntimeError('exit timeout')
    pump(.05)
   after=termios.tcgetattr(slave);pending=getattr(termios,'PENDIN',0);before[3]&=~pending;after[3]&=~pending
   results.append({'mode':mode,'reload':True,'draft':True,'cancelled':cancelled,'saved':saved,'history_unchanged':[x['message'] for x in map(json.loads,source.read_text().splitlines()) if x['type']=='message']==[entries[1]['message']],'restored':before==after,'exit':p.returncode})
  finally:
   if p.poll() is None:p.kill();p.wait()
   os.close(master);os.close(slave)
print(json.dumps(results))
