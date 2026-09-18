import fcntl, http.server, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, threading, time
pi=pathlib.Path(sys.argv[1]).resolve()
requests=[];thinking=threading.Event();arguments=threading.Event()
class Server(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def do_POST(self):
  requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
  self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
  def chunk(delta,finish=None):
   self.wfile.write(('data: '+json.dumps({'choices':[{'index':0,'delta':delta,'finish_reason':finish}]})+'\n\n').encode());self.wfile.flush()
  if len(requests)==1:
   chunk({'reasoning_content':'THINKING_VISIBLE'})
   if not thinking.wait(15):return
   chunk({'tool_calls':[{'index':0,'id':'tool-112','type':'function','function':{'name':'bash','arguments':'{"command":"cat progress.txt'}}]})
   if not arguments.wait(15):return
   chunk({'tool_calls':[{'index':0,'function':{'arguments':'; while [ ! -f release ]; do sleep 0.05; done; cat final.txt"}'}}]},'tool_calls')
  else:chunk({'content':'**ANSWER_DONE**\n\n```go\nprintln("ok")\n```'},'stop')
  self.wfile.write(b'data: [DONE]\n\n');self.wfile.flush()
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server);threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='pi-text112-') as tmp:
 root=pathlib.Path(tmp);agent=root/'agent';agent.mkdir();sessions=root/'sessions'
 (root/'progress.txt').write_text('PROGRESS_VISIBLE\n');(root/'final.txt').write_text(''.join('output %d\n'%i for i in range(15))+'FINAL_TOOL_OUTPUT\n')
 (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','hideThinkingBlock':False}))
 (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':True,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
 master,slave=pty.openpty();before=termios.tcgetattr(slave);fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,44,0,0))
 env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
 command=['node',str(pi/'packages/coding-agent/dist/cli.js'),'--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-extensions','--no-skills','--session-dir',str(sessions)]
 process=subprocess.Popen(command,stdin=slave,stdout=slave,stderr=slave,cwd=root,env=env);output=b''
 def wait_for(predicate):
  global output
  deadline=time.monotonic()+20
  while not predicate():
   if time.monotonic()>deadline:raise RuntimeError(repr(output))
   if select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
 def plain():return re.sub(rb'\x1b\[[0-?]*[ -/]*[@-~]',b'',output)
 try:
  wait_for(lambda:b'\x1b[?2004h' in output);time.sleep(.5);os.write(master,b'run the tool\r')
  wait_for(lambda:b'THINKING_VISIBLE' in plain());thinking.set()
  wait_for(lambda:b'cat progress.txt' in plain());arguments.set()
  wait_for(lambda:b'PROGRESS_VISIBLE' in plain());(root/'release').write_text('')
  wait_for(lambda:b'ANSWER_DONE' in plain());os.write(master,b'\x0f')
  wait_for(lambda:b'FINAL_TOOL_OUTPUT' in plain());time.sleep(.2);os.write(master,b'\x04')
  wait_for(lambda:process.poll() is not None)
  wire=json.dumps(requests[-1]['messages'])
  print(json.dumps({'thinking_before_tool':True,'arguments_before_execution':True,'progress_before_final':True,'expanded_output':True,'answer_visible':True,'model_context_clean':'[running]' not in wire and 'ctrl+o' not in wire,'exit_code':process.returncode,'terminal_restored':termios.tcgetattr(slave)==before}))
 finally:
  thinking.set();arguments.set();(root/'release').write_text('')
  if process.poll() is None:process.kill();process.wait()
  os.close(master);os.close(slave)
server.shutdown()
