import fcntl, http.server, json, os, pathlib, pty, re, select, signal, struct, subprocess, sys, tempfile, termios, threading, time
pi=pathlib.Path(sys.argv[1]).resolve()
outcomes={}
for mode in ['regular','fullscreen']:
 release=threading.Event()
 class Server(http.server.BaseHTTPRequestHandler):
  def log_message(self,*args): pass
  def do_POST(self):
   self.rfile.read(int(self.headers['Content-Length']));self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
   def chunk(text,finish=None):
    self.wfile.write(('data: '+json.dumps({'choices':[{'index':0,'delta':{'content':text},'finish_reason':finish}]})+'\n\n').encode());self.wfile.flush()
   chunk('```text\n'+'\n'.join('ROW%03d'%i for i in range(60))+'\n')
   if not release.wait(20):return
   chunk('STREAM_DONE\n```','stop');self.wfile.write(b'data: [DONE]\n\n');self.wfile.flush()
 server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server);threading.Thread(target=server.serve_forever,daemon=True).start()
 with tempfile.TemporaryDirectory(prefix='pi-layout113-') as tmp:
  root=pathlib.Path(tmp);agent=root/'agent';agent.mkdir()
  (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':mode,'fullscreenScrollbar':'always','fullscreenExitOutput':'resume-hint'}))
  (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
  master,slave=pty.openpty();before=termios.tcgetattr(slave);fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,60,0,0))
  env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
  cmd=['node',str(pi/'packages/coding-agent/dist/cli.js'),'--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-session','--no-tools','--no-extensions','--no-skills']
  process=subprocess.Popen(cmd,stdin=slave,stdout=slave,stderr=slave,cwd=root,env=env);output=b'';screen=[[' ']*60 for _ in range(24)];row=col=0;pending='';history=[]
  def feed(data):
   global screen,row,col,pending,history
   pending+=data.decode('utf8',errors='replace')
   while pending:
    if pending[0]=='\x1b':
     m=re.match(r'\x1b\[([0-?]*)([ -/]*)([@-~])',pending)
     osc=re.match(r'\x1b(?:\]|_).*?(?:\x07|\x1b\\)',pending,re.S)
     if osc:pending=pending[osc.end():];continue
     if not m:
      if len(pending)<2 or pending[1] in '[]_':break
      pending=pending[2:];continue
     params,_,final=m.groups();pending=pending[m.end():];nums=[int(x) if x else 0 for x in params.split(';')] if not params.startswith(('?','>','<','=')) else []
     n=(nums[0] if nums else 0) or 1
     if final in 'Hf':row=max(0,min(len(screen)-1,n-1));col=max(0,min(len(screen[0])-1,(nums[1] if len(nums)>1 else 1)-1))
     elif final=='A':row=max(0,row-n)
     elif final=='B':row=min(len(screen)-1,row+n)
     elif final=='C':col=min(len(screen[0])-1,col+n)
     elif final=='D':col=max(0,col-n)
     elif final=='G':col=max(0,min(len(screen[0])-1,n-1))
     elif final=='J':
      if nums and nums[0]==3:history=[]
      elif nums and nums[0]==2:screen=[[' ']*len(screen[0]) for _ in screen]
      else:
       screen[row][col:]=[' ']*(len(screen[0])-col)
       for i in range(row+1,len(screen)):screen[i]=[' ']*len(screen[0])
     elif final=='K':
      if nums and nums[0]==2:screen[row]=[' ']*len(screen[0])
      else:screen[row][col:]=[' ']*(len(screen[0])-col)
     continue
    c=pending[0];pending=pending[1:]
    if c=='\r':col=0
    elif c=='\n':
     row+=1
     if row>=len(screen):history.append(''.join(screen.pop(0)));screen.append([' ']*len(screen[0]));row=len(screen)-1
    elif c>=' ':
     screen[row][min(col,len(screen[0])-1)]=c;col=min(len(screen[0])-1,col+1)
  def text():return '\n'.join(''.join(l) for l in screen)
  def pump(seconds=.2):
   global output
   end=time.monotonic()+seconds
   while time.monotonic()<end:
    if select.select([master],[],[],.02)[0]:data=os.read(master,65536);output+=data;feed(data)
  def wait(predicate):
   end=time.monotonic()+15
   while not predicate():
    if time.monotonic()>end:raise RuntimeError(f'{mode}: screen {text()!r}, output {output[-2000:]!r}')
    pump(.05)
  try:
   wait(lambda:b'\x1b[?2004h' in output);pump(.5);os.write(master,b'browse\r');wait(lambda:'ROW059' in text());pump(.15)
   result={'tail_visible':True,'alternate_screen':b'\x1b[?1049h' in output}
   if mode=='fullscreen':
    os.write(master,b'\x1b[H');wait(lambda:'ROW000' in text());pump(.15);result['history_visible']='ROW059' not in text()
    release.set();pump(.8);result['append_keeps_history']='ROW000' in text() and 'STREAM_DONE' not in text()
    os.write(master,b'\x1b[F');wait(lambda:'STREAM_DONE' in text());result['end_restores_tail']=True
    os.write(master,b'\x1b[<64;2;2M');pump(.15)
   else:release.set();wait(lambda:'STREAM_DONE' in text())
   if mode=='regular':result['native_scrollback']='ROW000' in '\n'.join(history) and 'ROW020' in '\n'.join(history)
   pump(.2);edit_offset=len(output)
   os.write(master,b'DRAFT\x1b[D\x1b[D');wait(lambda:'DRAFT' in text());pump(.15)
   if mode=='regular':result['incremental_editor']=b'ROW000' not in output[edit_offset:] and b'\x1b[2J' not in output[edit_offset:]
   for rows,cols in [(12,28),(30,80),(8,18),(24,60)]:
    screen=[[' ']*cols for _ in range(rows)];row=col=0;fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',rows,cols,0,0));process.send_signal(signal.SIGWINCH);pump(.2);wait(lambda:'DRAFT' in text())
   if mode=='fullscreen':os.write(master,b'\x1b[F')
   wait(lambda:'STREAM_DONE' in text());os.write(master,b'X');pump(.2);result['resize_keeps_editor']='DRAXFT' in text();os.write(master,b'\x03');pump(.2);os.write(master,b'\x04');wait(lambda:process.poll() is not None);pump(.1)
   result['exit_code']=process.returncode;result['terminal_restored']=termios.tcgetattr(slave)==before;outcomes[mode]=result
  finally:
   release.set()
   if process.poll() is None:process.kill();process.wait()
   os.close(master);os.close(slave)
 server.shutdown()
print(json.dumps(outcomes))
