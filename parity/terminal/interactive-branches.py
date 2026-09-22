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
with tempfile.TemporaryDirectory(prefix='pi-interactive-branches-') as tmp:
    root=pathlib.Path(tmp).resolve();agent=root/'agent';agent.mkdir();sessions=root/'sessions';sessions.mkdir()
    (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','defaultThinkingLevel':'off','defaultProvider':'deepseek','defaultModel':'deepseek-v4-flash','compaction':{'enabled':False}}))
    models=[{'id':'deepseek-v4-'+kind,'name':kind,'contextWindow':65536,'maxTokens':4096,'reasoning':True,'thinkingLevelMap':{'high':'high','max':'max'},'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}} for kind in ['flash','pro']]
    (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':models}}}))
    # Persisted credentials allow reopening the Session without model flags.
    (agent/'auth.json').write_text(json.dumps({'deepseek':{'type':'api_key','key':'synthetic'}}))
    timestamp='2026-01-01T00:00:00.000Z'
    entries=[{'type':'session','version':3,'id':'source','timestamp':timestamp,'cwd':str(root)},
      {'type':'model_change','id':'m','parentId':None,'timestamp':timestamp,'provider':'deepseek','modelId':'deepseek-v4-flash'},
      {'type':'thinking_level_change','id':'t','parentId':'m','timestamp':timestamp,'thinkingLevel':'off'}]
    for i in [1,2]:
        entries.extend([{'type':'message','id':'u'+str(i),'parentId':'t' if i==1 else 'a1','timestamp':timestamp,'message':{'role':'user','content':[{'type':'text','text':f'question {i}'}],'timestamp':1767225600000}},
          {'type':'message','id':'a'+str(i),'parentId':'u'+str(i),'timestamp':timestamp,'message':{'role':'assistant','content':[{'type':'text','text':f'answer {i}'}],'api':'openai-completions','provider':'deepseek','model':'deepseek-v4-flash','usage':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'totalTokens':0,'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0,'total':0}},'stopReason':'stop','timestamp':1767225600000}}])
    source=sessions/'source.jsonl';source.write_text(''.join(json.dumps(e)+'\n' for e in entries));original=source.read_bytes()
    fork=None
    restored=[]
    def run(resume=False):
        master,slave=pty.openpty();before=termios.tcgetattr(slave)
        fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',32,100,0,0))
        env={'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
        if is_pig:
            env.update({'PIG_CODING_AGENT_DIR':str(agent),'PIG_OFFLINE':'1','DEEPSEEK_API_KEY':'synthetic','PIG_DEEPSEEK_BASE_URL':f'http://127.0.0.1:{server.server_port}'})
        args=([str(pi)] if is_pig else ['node',str(pi/'packages/coding-agent/dist/cli.js')])+['--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
        args+=['--session',str(fork if resume else source)]
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
            if resume:send(b'reopened question\r',b'DONE_6')
            else:
                wait(b'answer 2')
                send(b'/fork\r',b'Fork from Message')
                send(b'\x1b[27u');time.sleep(.2)
                send(b'/fork\r',b'Fork from Message')
                send(b'\r',b'Forked to new session');time.sleep(.3)
                send(b' revised\r',b'DONE_1')
                send(b'/tree\r',b'Session Tree')
                send(b'question 2 revised');send(b'\r',b'Summarize branch?')
                send(b'\r');time.sleep(.5)
                send(b' from tree\r',b'DONE_2')
                send(b'/tree\r',b'Session Tree')
                send(b'answer 1');send(b'L',b'Label (empty to remove):')
                send(b'checkpoint\r',b'checkpoint')
                send(b'\r',b'Summarize branch?')
                send(b'\x1b[27u',b'Session Tree')
                send(b'\r',b'Summarize branch?')
                send(b'\r');time.sleep(.5)
                send(b'new branch question\r',b'DONE_3')
                send(b'/tree\r',b'Session Tree')
                send(b'DONE_1');send(b'\r',b'Summarize branch?')
                send(b'\x1b[B');send(b'\r',b'Navigated to selected point');time.sleep(.2)
                send(b'after summary\r',b'DONE_5')
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
    run()
    forks=[p for p in sessions.rglob('*.jsonl') if p!=source]
    if len(forks)!=1:raise RuntimeError(f'expected one fork: {forks}')
    fork=forks[0];run(True)
    rows=[json.loads(line) for line in fork.read_text().splitlines()]
    by_id={e['id']:e for e in rows[1:]};branch=[];leaf=rows[-1]['id']
    while leaf:
        e=by_id[leaf];branch.append(e);leaf=e.get('parentId')
    branch.reverse()
    def text(content):
        return content if isinstance(content,str) else ''.join(p.get('text','') for p in content if p.get('type')=='text')
    print(json.dumps({'requests':[{'users':[text(m['content']) for m in req['messages'] if m['role']=='user'],'assistants':[text(m['content']) for m in req['messages'] if m['role']=='assistant']} for req in requests],
      'source_unchanged':source.read_bytes()==original,'parent_session':rows[0].get('parentSession')==str(source),
      'saved_users':[text(e['message']['content']) for e in rows if e['type']=='message' and e['message']['role']=='user'],
      'branch_users':[text(e['message']['content']) for e in branch if e['type']=='message' and e['message']['role']=='user'],
      'labels':[e.get('label') for e in rows if e['type']=='label'],
      'summaries':[e['summary'] for e in rows if e['type']=='branch_summary'],
      'terminal_restored':restored}))
server.shutdown();server.server_close()
