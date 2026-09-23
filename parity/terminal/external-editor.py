import signal, fcntl, http.server, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, threading, time
binary=pathlib.Path(sys.argv[1]).resolve(); pig='--pig' in sys.argv
requests=[]; release=threading.Event()
class Server(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def do_POST(self):
  requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length'])))); n=len(requests)
  self.send_response(200); self.send_header('Content-Type','text/event-stream'); self.end_headers()
  def chunk(text,finish=None):
   self.wfile.write(('data: '+json.dumps({'choices':[{'index':0,'delta':{'content':text},'finish_reason':finish}]})+'\n\n').encode()); self.wfile.flush()
  try:
   chunk('PARTIAL_'+str(n))
   if n==1: release.wait(15)
   chunk(' DONE_'+str(n),'stop'); self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
  except (BrokenPipeError,ConnectionResetError): pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='bash122-') as tmp:
 root=pathlib.Path(tmp); agent=root/'agent'; agent.mkdir(); sessions=root/'sessions'
 (agent/'settings.json').write_text(json.dumps({'externalEditor':str(root/'editor')+' arg  \"literal\"','quietStartup':True,'tuiMode':('fullscreen' if '--fullscreen' in sys.argv else 'regular'),'compaction':{'enabled':False},'retry':{'enabled':False}}))
 (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
 master,slave=pty.openpty(); before=termios.tcgetattr(slave); fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,120,0,0))
 env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1','PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1','PIG_DEEPSEEK_BASE_URL':f'http://127.0.0.1:{server.server_port}'}
 args=([str(binary)] if pig else ['node',str(binary/'packages/coding-agent/dist/cli.js')])+['--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
 def terminal_session():
  os.setsid(); fcntl.ioctl(slave,termios.TIOCSCTTY,0)
 supervisor=[sys.executable,'-c',"import json,pathlib,subprocess,sys,termios; code=subprocess.call(sys.argv[1:]); pathlib.Path('terminal-after.json').write_text(json.dumps(termios.tcgetattr(0),default=lambda v:list(v))); sys.exit(code)"]
 p=subprocess.Popen(supervisor+args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root,preexec_fn=terminal_session); output=b''
 def pump(seconds=.1):
  global output
  end=time.monotonic()+seconds
  while time.monotonic()<end:
   if select.select([master],[],[],.02)[0]: output+=os.read(master,65536)
 def wait(check):
  end=time.monotonic()+15
  while not check():
   if time.monotonic()>end or p.poll()!=None: raise RuntimeError(f'timeout ({p.poll()}): {output[-6000:]!r}')
   pump(.03)
 def send(data):
  global output
  output=b''; os.write(master,data); pump(.12)
 def command(text,marker):
  send(text.encode()); send(b'\r'); wait(lambda:marker.encode() in output); pump(.15)
 try:
  wait(lambda:b'> ' in output if pig else b'deepseek-v4-flash' in output); pump(.6)
  command('background question','PARTIAL_1')
  draft='本次草稿 🐷\nsecond line'
  edited='外部编辑 ✅\nsecond changed'
  editor=root/'editor'
  editor.write_text("#!/usr/bin/env python3\nimport json,os,pathlib,sys,termios\np=pathlib.Path(sys.argv[-1]); r=pathlib.Path.cwd()\n(r/'capture.json').write_text(json.dumps({'text':p.read_text(),'args':sys.argv[1:-1],'path':str(p),'pid':os.getpid(),'cooked':bool(termios.tcgetattr(0)[3]&termios.ICANON)}))\nprint('EDITOR_READY',flush=True)\ninput()\nmode=(r/'action').read_text()\nif mode=='read-fail':p.unlink()\nelif mode=='success':p.write_text('外部编辑 ✅\\nsecond changed\\n')\nelse:sys.exit(int(mode))\n")
  if '--vim' in sys.argv:
   editor.write_text(editor.read_text().replace("elif mode=='success':p.write_text('外部编辑 ✅\\nsecond changed\\n')","elif mode=='success':os.execvp('vim',['vim','-Nu','NONE','-n',str(p)])"))
  editor.chmod(0o700)
  (root/'action').write_text('success')
  # Settings are loaded at startup; the executable itself can appear afterwards.
  send(b'\x1b[200~'+draft.encode()+b'\x1b[201~'); send(b'\x07')
  wait(lambda:b'EDITOR_READY' in output)
  captured=json.loads((root/'capture.json').read_text())
  output=b''; release.set(); pump(.5)
  background_quiet=output==b''
  send(b'\n')
  if '--vim' in sys.argv:
   wait(lambda:b'prompt.md' in output); pump(.2)
   send(b'ggdGi'+edited.encode()+b'\x1b:wq\r')
  wait(lambda:b'\x1b[?2004h' in output); pump(.3)
  cleaned=not pathlib.Path(captured['path']).parent.exists()
  send(b'\r'); wait(lambda:b'DONE_2' in output)
  outcomes=[]
  for action in ['7','130','missing']+(['read-fail'] if '--failures' in sys.argv else []):
   (root/'action').write_text(action)
   original='retained '+action
   if action=='missing':editor.rename(root/'saved-editor')
   send(original.encode()); send(b'\x07')
   if action!='missing':
    wait(lambda:b'EDITOR_READY' in output)
    capture=json.loads((root/'capture.json').read_text())
    send(b'\n')
   else: pump(.5)
   wait(lambda:original.encode() in output)
   if pig: wait(lambda:b'Error:' in output)
   pump(.3)
   if action!='missing':cleaned=cleaned and not pathlib.Path(capture['path']).parent.exists()
   send(b'\r'); wait(lambda:('DONE_'+str(len(outcomes)+3)).encode() in output)
   outcomes.append(action)
   if action=='missing':(root/'saved-editor').rename(editor)
  send(b'\x04'); wait(lambda:p.poll()!=None)
  after=json.loads((root/'terminal-after.json').read_text()); before=json.loads(json.dumps(before,default=lambda v:list(v))); pending=getattr(termios,'PENDIN',0); before[3]&=~pending; after[3]&=~pending
  def text(m):
   c=m['content']; return c if isinstance(c,str) else ''.join(b.get('text','') for b in c)
  prompts=[text([m for m in r['messages'] if m['role']=='user'][-1]) for r in requests]
  result={'background_quiet':background_quiet,'initial':captured['text'],'args':captured['args'],'cooked':captured['cooked'],'prompts':prompts,'cleaned':cleaned,'exit_code':p.returncode,'restored':before==after}
  print(json.dumps(result))
 finally:
  release.set()
  if p.poll() is None:
   try:
    capture=json.loads((root/'capture.json').read_text()); os.killpg(capture['pid'],signal.SIGKILL)
   except (FileNotFoundError,ProcessLookupError):pass
   os.killpg(p.pid,signal.SIGKILL);p.wait()
  os.close(master);os.close(slave)
server.shutdown();server.server_close()
