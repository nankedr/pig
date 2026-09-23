import fcntl, http.server, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, threading, time
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
   if n==2: release.wait(15)
   chunk(' DONE_'+str(n),'stop'); self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
  except (BrokenPipeError,ConnectionResetError): pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='bash122-') as tmp:
 root=pathlib.Path(tmp); agent=root/'agent'; agent.mkdir(); sessions=root/'sessions'
 (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','compaction':{'enabled':False},'retry':{'enabled':False}}))
 (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
 master,slave=pty.openpty(); before=termios.tcgetattr(slave); fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,120,0,0))
 env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1','PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1','PIG_DEEPSEEK_BASE_URL':f'http://127.0.0.1:{server.server_port}'}
 args=([str(binary)] if pig else ['node',str(binary/'packages/coding-agent/dist/cli.js')])+['--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
 p=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root); output=b''
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
 def messages():
  return [row['message'] for path in sessions.rglob('*.jsonl') for line in path.read_text().splitlines() if (row:=json.loads(line))['type']=='message']
 try:
  wait(lambda:b'> ' in output if pig else b'deepseek-v4-flash' in output); pump(.6)
  command("!printf 'IN_%s\\n' CONTEXT; printf 'ERR_%s\\n' STREAM >&2; exit 7",'IN_CONTEXT')
  wait(lambda:b'ERR_STREAM' in output and (b'exit 7' in output or b'code 7' in output))
  command("!!printf 'OUT_%s\\n' CONTEXT",'OUT_CONTEXT')
  command('first question','DONE_1')
  command('second question','PARTIAL_2')
  command("!printf 'DURING_%s\\n' AGENT",'DURING_AGENT')
  send(b'queued question\x1b\r'); wait(lambda:b'Follow-up: queued question' in output)
  release.set(); wait(lambda:b'DONE_3' in output); pump(.3)
  command("!printf 'CANCEL_%s\\n' PART; exec sleep 30",'CANCEL_PART')
  send(b'!touch forbidden\r'); wait(lambda:b'already running' in output)
  send(b'\x03'); send(b'\x1b[27u'); wait(lambda:b'cancelled' in output); pump(.2)
  command("!i=0; while [ $i -lt 2100 ]; do printf 'LINE_%s\\n' $i; i=$((i+1)); done",'LINE_2099')
  wait(lambda:b'Output truncated. Full output:' in output)
  send(b'\x0f'); pump(.2)
  command('after question','DONE_4'); pump(.3)
  ms=messages(); bash=[m for m in ms if m['role']=='bashExecution']
  full=pathlib.Path(bash[-1]['fullOutputPath']).read_text()
  send(b'\x04'); wait(lambda:p.poll()!=None)
  after=termios.tcgetattr(slave); pending=getattr(termios,'PENDIN',0); before[3]&=~pending; after[3]&=~pending
  def text(m):
   c=m['content']; return c if isinstance(c,str) else ''.join(b.get('text','') for b in c)
  contexts=['\n'.join(text(m) for m in r['messages'] if m['role']=='user') for r in requests]
  result={'bash':[{'command':m['command'],'output':m['output'] if not m.get('truncated') else 'TAIL','exit':m.get('exitCode'),'cancelled':m['cancelled'],'truncated':m['truncated'],'excluded':m.get('excludeFromContext',False)} for m in bash], 'context_included':['IN_CONTEXT' in c for c in contexts], 'context_excluded':all('OUT_CONTEXT' not in c for c in contexts),'deferred_context':['DURING_AGENT' in c for c in contexts], 'cancel_in_context':'CANCEL_PART' in contexts[-1], 'queued':any('queued question' in c for c in contexts),'mutual_exclusion':not (root/'forbidden').exists(),'full_output':full.startswith('LINE_0\n') and full.endswith('LINE_2099\n'),'exit_code':p.returncode,'restored':before==after}
  print(json.dumps(result))
  pathlib.Path(bash[-1]['fullOutputPath']).unlink()
 finally:
  release.set()
  if p.poll() is None:p.kill();p.wait()
  os.close(master);os.close(slave)
server.shutdown();server.server_close()
