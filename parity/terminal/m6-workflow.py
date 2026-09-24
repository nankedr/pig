import base64, fcntl, http.server, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, threading, time
binary=pathlib.Path(sys.argv[1]).resolve(); pig='--pig' in sys.argv or '--sdk' in sys.argv
mode='fullscreen' if '--fullscreen' in sys.argv else 'regular'
artifacts=pathlib.Path(os.environ['PIG_M6_ARTIFACTS']).resolve() if os.environ.get('PIG_M6_ARTIFACTS') else None
if artifacts: artifacts.mkdir(parents=True,exist_ok=True)
requests=[]; release=threading.Event(); queue_release=threading.Event(); summary_mode='success'
def text(m):
 c=m.get('content',''); return c if isinstance(c,str) else ''.join(b.get('text','') for b in c)
class Server(http.server.BaseHTTPRequestHandler):
 def log_message(self,*args): pass
 def do_POST(self):
  req=json.loads(self.rfile.read(int(self.headers['Content-Length']))); requests.append(req)
  summary='context summarization assistant' in text(req['messages'][0]); mode=summary_mode
  if summary and mode=='failure':
   self.send_response(400); self.end_headers(); self.wfile.write(b'{"error":{"message":"SUMMARY_FAILURE"}}'); return
  self.send_response(200); self.send_header('Content-Type','text/event-stream'); self.end_headers()
  try:
   if summary and mode=='cancel': release.wait(20)
   if len(requests)==11:
    self.wfile.write(b'data: {"choices":[{"index":0,"delta":{"content":"QUEUE_PARTIAL"}}]}\n\n');self.wfile.flush()
    queue_release.wait(20)
   value='SUMMARY_MARKER' if summary else 'RESPONSE_'+str(len(requests))
   data={'choices':[{'index':0,'delta':{'content':value},'finish_reason':'stop'}],'usage':{'prompt_tokens':120,'completion_tokens':20,'total_tokens':140}}
   self.wfile.write(('data: '+json.dumps(data)+'\n\ndata: [DONE]\n\n').encode()); self.wfile.flush()
  except (BrokenPipeError,ConnectionResetError): pass
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='m6-workflow-') as tmp:
 root=pathlib.Path(tmp); agent=root/'agent'; agent.mkdir(); sessions=root/'sessions'
 (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':mode,'theme':'local','externalEditor':str(root/'editor'),'compaction':{'enabled':False,'keepRecentTokens':10,'reserveTokens':1000},'retry':{'enabled':False}}))
 (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
 (agent/'themes').mkdir()
 theme=json.loads((pathlib.Path(__file__).resolve().parents[2]/'codingagent/themes/dark.json').read_text());theme['name']='local'
 for key in theme['colors']:theme['colors'][key]='#112233'
 theme_path=agent/'themes/local.json';theme_path.write_text(json.dumps(theme))
 editor=root/'editor';editor.write_text('#!/usr/bin/env python3\nimport pathlib,sys\np=pathlib.Path(sys.argv[-1]);pathlib.Path("editor-input.txt").write_text(p.read_text());p.write_text("edited question with enough context for compaction\\n")\n');editor.write_text(editor.read_text().replace('pathlib.Path("editor-input.txt")', 'pathlib.Path('+repr(str(root/'editor-input.txt'))+')'));editor.chmod(0o700)
 master,slave=pty.openpty(); before=termios.tcgetattr(slave); fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,120,0,0))
 env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','COLORTERM':'truecolor','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1','PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1','PIG_DEEPSEEK_BASE_URL':f'http://127.0.0.1:{server.server_port}'}
 args=([str(binary)] if pig else ['node',str(binary/'packages/coding-agent/dist/cli.js')])+['--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--session-dir',str(sessions)]
 if '--sdk' in sys.argv: args=[str(binary)]
 p=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root); output=b''; transcript=bytearray(); screens={}
 def pump(seconds=.1):
  global output
  end=time.monotonic()+seconds
  while time.monotonic()<end:
   if select.select([master],[],[],.02)[0]:
    data=os.read(master,65536);output+=data;transcript.extend(data)
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
  wait(lambda:b'\x1b[38;2;17;34;51m' in output)
  send(b'DRAFT_THEME')
  for key in theme['colors']:theme['colors'][key]='#445566'
  theme_path.write_text(json.dumps(theme));wait(lambda:b'\x1b[38;2;68;85;102m' in output and b'DRAFT_THEME' in output)
  screens['theme']=output.decode(errors='replace');send(b'\x03')
  command('/settings','Type to search');send(b'Quiet startup');send(b'\r')
  wait(lambda:json.loads((agent/'settings.json').read_text()).get('quietStartup')==False)
  send(b'\x1b[27u')
  command('/model','deepseek-v4-flash');send(b'\x1b[27u');pump(.3)
  command('!!printf BASH_COMBINED','BASH_COMBINED');pump(.3)
  send(b'\x1b[200~draft line one\nline two\x1b[201~');send(b'\x07')
  wait(lambda:b'edited question with enough context' in output)
  screens['editor']=output.decode(errors='replace')
  send(b'\r');wait(lambda:b'RESPONSE_1' in output);pump(.3)
  edited_input=(root/'editor-input.txt').read_text()=='draft line one\nline two'

  command('second question with enough context for retention','RESPONSE_2')
  command('/resume','Session');send(b'\x1b[27u');pump(.3)
  command('/session','Session Info'); stats_visible=b'Input:' in output and b'Output:' in output
  if pig: assert b'Input: 240' in output and b'Output: 40' in output and b'Total: 280' in output, output
  command('/compact preserve important details','Compacted'); pump(.3)
  summary_visible=b'SUMMARY_MARKER' in output; send(b'\x0f'); pump(.2); summary_visible=summary_visible or b'SUMMARY_MARKER' in output
  
  if pig:
   command('/session','Session Info'); assert b'Tokens: unknown /' in output and b'Total: 420' in output, output
  command('after compact','RESPONSE_4'); compact_context='SUMMARY_MARKER' in json.dumps(requests[-1]['messages'])
  (agent/'prompts').mkdir(); (agent/'prompts'/'fresh.md').write_text('RELOADED_TEMPLATE $1')
  (agent/'skills'/'fresh').mkdir(parents=True); (agent/'skills'/'fresh'/'SKILL.md').write_text('---\nname: fresh\ndescription: Fresh skill\n---\nRELOADED_SKILL')
  (root/'AGENTS.md').write_text('RELOADED_CONTEXT')
  command('/reload','Reloaded'); reload_visible=True
  command('/fresh target','RESPONSE_5'); template_context='RELOADED_TEMPLATE target' in json.dumps(requests[-1]['messages']); context_reloaded='RELOADED_CONTEXT' in json.dumps(requests[-1]['messages'])
  command('/skill:fresh','RESPONSE_6'); skill_context='RELOADED_SKILL' in json.dumps(requests[-1]['messages'])
  command('/export "report space.html"','Session exported to:'); html=(root/'report space.html').read_text(); exported_data=json.loads(base64.b64decode(re.search(r'<script id="session-data" type="application/json">(.*?)</script>',html,re.S).group(1))); exported='SUMMARY_MARKER' in json.dumps(exported_data)
  command('/export missing/report.html','Failed to export session:')
  summary_mode='failure'
  command('/compact','Compaction failed:')
  command('after failure','RESPONSE_8')
  summary_mode='cancel'; send(b'/compact\r'); wait(lambda:len(requests)==9); pump(.3)
  send(b'\x1b[27u'); wait(lambda:b'Compaction cancelled' in output or b'operation was aborted' in output); release.set(); pump(.2)
  command('after cancel','RESPONSE_10')
  screens['maintenance']=output.decode(errors='replace')
  command('queue initial','QUEUE_PARTIAL')
  send(b'follow queued\x1b\r');wait(lambda:b'Follow-up: follow queued' in output)
  send(b'steer queued\r');wait(lambda:b'Steering: steer queued' in output)
  queue_release.set();wait(lambda:b'RESPONSE_13' in output);pump(.3)
  command('after queues','RESPONSE_14')
  screens['queues']=output.decode(errors='replace')
  fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,72,0,0));pump(.4)
  send(b'\x1b[5~');send(b'\x1b[6~');pump(.2)
  command('/session','Session Info');screens['resized']=output.decode(errors='replace')
  ms=messages(); users=[text(m) for m in ms if m['role']=='user']
  entries=[json.loads(line) for pth in sessions.rglob('*.jsonl') for line in pth.read_text().splitlines()]
  compactions=[e for e in entries if e['type']=='compaction']
  send(b'\x04'); wait(lambda:p.poll()!=None)
  after=termios.tcgetattr(slave); pending=getattr(termios,'PENDIN',0); before[3]&=~pending; after[3]&=~pending
  result={'mode':mode,'edited_input':edited_input,'theme_reloaded':True,'setting_saved':json.loads((agent/'settings.json').read_text())['quietStartup']==False,'bash_recorded':any(m['role']=='bashExecution' and 'BASH_COMBINED' in m.get('output','') and m.get('excludeFromContext') for m in ms),'stats_visible':stats_visible,'summary_visible':summary_visible,'compact_context':compact_context,'reload_visible':reload_visible,'template_context':template_context,'context_reloaded':context_reloaded,'skill_context':skill_context,'exported':exported,'compactions':[e['summary'] for e in compactions],'commands_not_prompts':all(not u.startswith(('/compact','/reload','/session','/export')) for u in users),'queued_in_order':users[-4:-1]==['queue initial','steer queued','follow queued'],'continued':users[-1]=='after queues','exit_code':p.returncode,'restored':before==after}
  if artifacts:
   (artifacts/(mode+'.ansi')).write_bytes(transcript)
   (artifacts/(mode+'-screens.json')).write_text(json.dumps(screens,ensure_ascii=False,indent=2)+'\n')
   (artifacts/(mode+'-session.json')).write_text(json.dumps(entries,ensure_ascii=False,indent=2)+'\n')
   (artifacts/(mode+'-outcome.json')).write_text(json.dumps(result,indent=2)+'\n')
   (artifacts/(mode+'-settings.json')).write_bytes((agent/'settings.json').read_bytes())
   (artifacts/(mode+'-export.html')).write_text(html)
  print(json.dumps(result))

 finally:
  if artifacts:
   (artifacts/(mode+'-requests.json')).write_text(json.dumps(requests,ensure_ascii=False,indent=2)+'\n')
   (artifacts/(mode+'.ansi')).write_bytes(transcript)
  release.set();queue_release.set()
  if p.poll() is None:p.kill();p.wait()
  os.close(master);os.close(slave)
server.shutdown();server.server_close()
