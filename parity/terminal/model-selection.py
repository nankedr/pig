import fcntl, http.server, json, os, pathlib, pty, select, struct, subprocess, sys, tempfile, termios, threading, time
pi=pathlib.Path(sys.argv[1]).resolve()
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
    root=pathlib.Path(tmp);agent=root/'agent';agent.mkdir();sessions=root/'sessions'
    (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','defaultThinkingLevel':'high','compaction':{'enabled':False}}))
    models=[{'id':'deepseek-v4-'+kind,'name':kind,'contextWindow':65536,'maxTokens':4096,'reasoning':True,'thinkingLevelMap':{'high':'high','max':'max'},'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}} for kind in ['flash','pro']]
    (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':models}}}))
    # Persisted credentials allow reopening the Session without model flags.
    (agent/'auth.json').write_text(json.dumps({'deepseek':{'type':'api_key','key':'synthetic'}}))
    restored=[]
    def run(resume=False):
        master,slave=pty.openpty();before=termios.tcgetattr(slave)
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0))
        env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
        args=['node',str(pi/'packages/coding-agent/dist/cli.js'),'--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
        args+=['--session',str(next(sessions.rglob('*.jsonl')))] if resume else ['--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic']
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
            if resume:send(b'restored question\r',b'DONE_4')
            else:
                send(b'before question\r',b'DONE_1')
                send(b'/model\r',b'Model Name:')
                send(b'v4-pro\r',b'Model: deepseek-v4-pro')
                send(b'\x1b[Z');time.sleep(.2)
                send(b'/scoped-models\r',b'Model Configuration')
                send(b'flash');send(b'\r',b'unsaved')
                send(b'\x13',b'Model selection saved to settings')
                send(b'\x1b[27u');time.sleep(.2)
                send(b'\x10');time.sleep(.2)
                send(b'after question\r',b'DONE_2')
                send(b'/model\r',b'Model Name:')
                send(b'\x1b[27u');time.sleep(.2)
                send(b'cancel question\r',b'DONE_3')
            time.sleep(.3);os.write(master,b'\x04')
            end=time.monotonic()+10
            while process.poll() is None:
                if time.monotonic()>end:raise RuntimeError('exit timeout')
                if select.select([master],[],[],.05)[0]:output+=os.read(master,65536)
            restored.append(termios.tcgetattr(slave)==before)
            if process.returncode:raise RuntimeError(f'exit {process.returncode}')
        finally:
            if process.poll() is None:process.kill();process.wait()
            os.close(master);os.close(slave)
    run();run(True)
    settings=json.loads((agent/'settings.json').read_text())
    entries=[json.loads(line) for path in sessions.rglob('*.jsonl') for line in path.read_text().splitlines()]
    configs=[{'model':row['modelId']} if row['type']=='model_change' else {'thinking':row['thinkingLevel']} for row in entries if row['type'] in ['model_change','thinking_level_change']]
    print(json.dumps({'requests':[{'model':r['model'],'thinking':r.get('reasoning_effort'),'enabled':r.get('thinking',{}).get('type')} for r in requests],'configs':configs,'settings':{k:settings.get(k) for k in ['defaultProvider','defaultModel','defaultThinkingLevel','enabledModels']},'terminal_restored':restored}))
server.shutdown();server.server_close()
