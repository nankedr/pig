import fcntl, http.server, json, os, pathlib, pty, select, struct, subprocess, sys, tempfile, termios, threading, time
pi=pathlib.Path(sys.argv[1]).resolve()
is_pig='--pig' in sys.argv
requests=[]
class Server(http.server.BaseHTTPRequestHandler):
    def log_message(self,*args): pass
    def do_POST(self):
        requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
        self.send_response(200);self.send_header('Content-Type','text/event-stream');self.end_headers()
        payload={'choices':[{'index':0,'delta':{'content':'DONE_'+str(len(requests))},'finish_reason':'stop'}]}
        self.wfile.write(('data: '+json.dumps(payload)+'\n\ndata: [DONE]\n\n').encode());self.wfile.flush()
server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Server)
threading.Thread(target=server.serve_forever,daemon=True).start()
with tempfile.TemporaryDirectory(prefix='pi-model-selection-') as tmp:
    root=pathlib.Path(tmp).resolve();agent=root/'agent';agent.mkdir();sessions=root/'sessions';sessions.mkdir()
    (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','defaultThinkingLevel':'off','defaultProvider':'deepseek','defaultModel':'deepseek-v4-flash','compaction':{'enabled':False}}))
    models=[{'id':'deepseek-v4-'+kind,'name':kind,'contextWindow':65536,'maxTokens':4096,'reasoning':True,'thinkingLevelMap':{'high':'high','max':'max'},'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}} for kind in ['flash','pro']]
    (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':models}}}))
    # Persisted credentials allow reopening the Session without model flags.
    (agent/'auth.json').write_text(json.dumps({'deepseek':{'type':'api_key','key':'synthetic'}}))
    for name,model,thinking in [('source','flash','off'),('target','pro','high')]:
        timestamp='2026-01-01T00:00:00.000Z'
        entries=[{'type':'session','version':3,'id':name,'timestamp':timestamp,'cwd':str(root)},
          {'type':'model_change','id':'m','parentId':None,'timestamp':timestamp,'provider':'deepseek','modelId':'deepseek-v4-'+model},
          {'type':'thinking_level_change','id':'t','parentId':'m','timestamp':timestamp,'thinkingLevel':thinking},
          {'type':'session_info','id':'n','parentId':'t','timestamp':timestamp,'name':name+' saved'},
          {'type':'message','id':'u','parentId':'n','timestamp':timestamp,'message':{'role':'user','content':[{'type':'text','text':name+' history'}],'timestamp':1767225600000}},
          {'type':'message','id':'a','parentId':'u','timestamp':timestamp,'message':{'role':'assistant','content':[{'type':'text','text':name+' answer'}],'api':'openai-completions','provider':'deepseek','model':'deepseek-v4-'+model,'usage':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'totalTokens':0,'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'total':0}},'stopReason':'stop','timestamp':1767225600000}}]
        (sessions/(name+'.jsonl')).write_text(''.join(json.dumps(e)+'\n' for e in entries))
    restored=[]
    def run(resume=False):
        master,slave=pty.openpty();before=termios.tcgetattr(slave)
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0))
        env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
        if is_pig:
            env.update({'PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1','DEEPSEEK_API_KEY':'synthetic','PIG_DEEPSEEK_BASE_URL':f'http://127.0.0.1:{server.server_port}'})
        args=([str(pi)] if is_pig else ['node',str(pi/'packages/coding-agent/dist/cli.js')])+['--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
        args+=['--session',str(sessions/'target.jsonl')] if resume else ['--resume']
        process=subprocess.Popen(args,stdin=slave,stdout=slave,stderr=slave,env=env,cwd=root)
        output=b''
        def wait(needle):
            nonlocal output
            end=time.monotonic()+20
            while needle not in output:
                if time.monotonic()>end or process.poll() is not None:raise RuntimeError(f'missing {needle!r}: {output!r}')
                if select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
        def send(data,needle=None):
            nonlocal output
            output=b'';os.write(master,data);time.sleep(.15)
            if needle:wait(needle)
        try:
            wait(b'\x1b[?2004h');time.sleep(.5)
            if resume:send(b'reopened question\r',b'DONE_5')
            else:
                wait(b'target saved')
                send(b'target');time.sleep(.3)
                send(b'\r',b'target answer');time.sleep(1)
                send(b'resumed question\r',b'DONE_1')
                send(b'/resume\r',b'Resume Session')
                send(b'\x1b[27u');time.sleep(.2)
                send(b'cancel question\r',b'DONE_2')
                send(b'/new\r',b'Started new session' if is_pig else b'New session started');time.sleep(.5)
                send(b'new question\r',b'DONE_3')
                send(b'/resume\r',b'Resume Session')
                send(b'source');time.sleep(.3)
                send(b'\r',b'source answer');time.sleep(.5)
                send(b'switched question\r',b'DONE_4')
            time.sleep(.3);os.write(master,b'\x04')
            end=time.monotonic()+10
            while process.poll() is None:
                if time.monotonic()>end:raise RuntimeError('exit timeout')
                if select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
            after=termios.tcgetattr(slave)
            # PENDIN is the kernel's transient retype-pending bit, not an application terminal mode.
            pending=getattr(termios,'PENDIN',0)
            before[3]&=~pending;after[3]&=~pending
            restored.append(after==before)
            if process.returncode:raise RuntimeError(f'exit {process.returncode}')
        finally:
            if process.poll() is None:process.kill();process.wait()
            os.close(master);os.close(slave)
    run();run(True)
    saved={}
    for path in sessions.rglob('*.jsonl'):
        rows=[json.loads(line) for line in path.read_text().splitlines()]
        name=path.stem if path.stem in ['source','target'] else 'new'
        def text(message):
            c=message.get('content',[])
            return c if isinstance(c,str) else ''.join(x.get('text','') for x in c if x.get('type')=='text')
        saved[name]={'name':next((row['name'] for row in reversed(rows) if row['type']=='session_info'),None),'users':[text(row['message']) for row in rows if row['type']=='message' and row['message']['role']=='user']}
    print(json.dumps({'requests':[{'model':r['model'],'thinking':r.get('reasoning_effort'),'users':[m['content'] for m in r['messages'] if m['role']=='user']} for r in requests],'sessions':saved,'terminal_restored':restored}))
server.shutdown();server.server_close()
