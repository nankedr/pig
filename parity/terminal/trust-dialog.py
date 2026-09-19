import fcntl, http.server, json, os, pathlib, pty, select, struct, subprocess, sys, tempfile, termios, threading, time
pi=pathlib.Path(sys.argv[1]).resolve()
requests=[]
class Server(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def do_POST(self):
  requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  self.wfile.write(b'data: {"choices":[{"delta":{"content":"TRUST_REPLY"},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
results=[]
try:
 for name,keys in [('trust',b'\r'),('parent',b'j\r'),('session',b'jj\r'),('deny',b'jjj\r'),('deny-session',b'jjjj\r'),('cancel',b'\x1b')]:
  with tempfile.TemporaryDirectory(prefix='pi-trust-dialog-') as temp:
   root=pathlib.Path(temp).resolve();cwd=root/'project';cwd.mkdir();agent=root/'agent';agent.mkdir();config=cwd/'.pi';config.mkdir()
   (config/'SYSTEM.md').write_text('PROJECT_TRUST_SYSTEM')
   (cwd/'AGENTS.md').write_text('CONTEXT_TRUST_EXCEPTION')
   (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular'}))
   (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}'}}}))
   runs=[]
   for attempt in range(2):
    master,slave=pty.openpty();before=termios.tcgetattr(slave);fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0))
    env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
    process=subprocess.Popen(['node',str(pi/'packages/coding-agent/dist/cli.js'),'--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--no-skills','--no-session'],cwd=cwd,env=env,stdin=slave,stdout=slave,stderr=slave)
    output=b''
    def wait_for(predicate):
     global output
     end=time.monotonic()+20
     while not predicate():
      if time.monotonic()>end or process.poll() is not None: raise RuntimeError(f'{name} PTY failed {output!r}')
      if select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
    try:
     prompt=attempt==0 or name in ['session','deny-session','cancel']
     if prompt:
      wait_for(lambda:b'Trust project folder?' in output);os.write(master,keys if attempt==0 else b'\x1b')
     wait_for(lambda:b'deepseek-v4-flash' in output)
     time.sleep(.2);os.write(master,b'hello\r');wait_for(lambda:b'TRUST_REPLY' in output)
     message=json.dumps(requests[-1]['messages']);os.write(master,b'\x04')
     wait_for(lambda:process.poll() is not None)
     while select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
     runs.append({'prompt':b'Trust project folder?' in output,'project': 'PROJECT_TRUST_SYSTEM' in message,'context':'CONTEXT_TRUST_EXCEPTION' in message,'exit':process.returncode,'restored':termios.tcgetattr(slave)==before})
    finally:
     if process.poll() is None:process.kill();process.wait()
     os.close(master);os.close(slave)
   saved=json.loads((agent/'trust.json').read_text()) if (agent/'trust.json').exists() else {}
   results.append({'name':name,'runs':runs,'saved':saved.get(str(cwd),saved.get(str(root))),'parent':str(root) in saved})
 print(json.dumps(results))
finally:server.shutdown()
