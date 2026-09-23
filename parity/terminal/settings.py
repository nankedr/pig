import fcntl, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, time
binary=pathlib.Path(sys.argv[1]).resolve();pig='--pig' in sys.argv
cases=[('Auto-compact','compaction.enabled'),('Skill commands','enableSkillCommands'),('Show hardware cursor','showHardwareCursor'),('Editor padding','editorPaddingX'),('Output padding','outputPad'),('Autocomplete max items','autocompleteMaxVisible'),('Clear on shrink','terminal.clearOnShrink'),('Terminal progress','terminal.showTerminalProgress'),('Steering mode','steeringMode'),('Follow-up mode','followUpMode'),('Hide thinking','hideThinkingBlock'),('Quiet startup','quietStartup'),('Default project trust','defaultProjectTrust'),('Double-escape action','doubleEscapeAction'),('Tree filter mode','treeFilterMode'),('TUI mode','tuiMode'),('Fullscreen exit output','fullscreenExitOutput'),('Fullscreen scrollbar','fullscreenScrollbar')]
results=[]
with tempfile.TemporaryDirectory(prefix='settings121-') as tmp:
 root=pathlib.Path(tmp);agent=root/'agent';agent.mkdir();settings=agent/'settings.json'
 settings.write_text(json.dumps({'theme':'dark','quietStartup':False,'compaction':{'enabled':True},'outputPad':1}))
 def stored(path):
  value=json.loads(settings.read_text())
  for key in path.split('.'):value=value.get(key) if isinstance(value,dict) else None
  return value
 def run(restart):
  master,slave=pty.openpty();before=termios.tcgetattr(slave);fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',40,110,0,0))
  env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1','PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1'}
  args=([str(binary)] if pig else ['node',str(binary/'packages/coding-agent/dist/cli.js')])+['--no-session','--no-tools','--no-extensions','--no-skills','--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic']
  p=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root);output=b''
  def pump(seconds=.3):
   nonlocal output
   end=time.monotonic()+seconds
   while time.monotonic()<end:
    if select.select([master],[],[],.02)[0]:output+=os.read(master,65536)
  def send(data):
   nonlocal output
   output=b'';os.write(master,data);pump()
  def wait(check):
   end=time.monotonic()+10
   while not check():
    if time.monotonic()>end or p.poll()!=None:raise RuntimeError(f'timeout: {re.sub(rb'\x1b\[[0-9;?]*[a-zA-Z]',b'',output)[-1500:]!r}')
    pump(.05)
  try:
   wait(lambda: b'> ' in output if pig else b'deepseek-v4-flash' in output);pump(.6)
   send(b'/settings');send(b'\r');wait(lambda:b'Type to search' in output)
   for label,path in cases:
    send(b'\x15'+label.encode());wait(lambda:label.encode() in output)
    if restart:
     saved=stored(path);value={'ask':'Ask','always':'Always trust','never':'Never trust'}[saved] if path=='defaultProjectTrust' else str(saved).lower() if isinstance(saved,bool) else str(saved)
     plain=re.sub(rb'\x1b\[[0-9;?]*[a-zA-Z]',b'',output).decode(errors='replace')
     if not re.search(re.escape(label)+r'\s+'+re.escape(value),plain):raise RuntimeError(f'restored {label}={value} not displayed: {plain}')
    else:
     old=stored(path);send(b'\r');wait(lambda:stored(path)!=old)
     results.append({'label':label,'value':stored(path)})
   send(b'\x1b[27u');send(b'\x04')
   wait(lambda:p.poll()!=None)
   after=termios.tcgetattr(slave);pending=getattr(termios,'PENDIN',0);before[3]&=~pending;after[3]&=~pending
   if before!=after or p.returncode!=0:raise RuntimeError('terminal restoration/exit failed')
  finally:
   if p.poll() is None:p.kill();p.wait()
   os.close(master);os.close(slave)
 run(False);run(True)
print(json.dumps({'changes':results,'restart':True,'restored':True}))
