import fcntl, http.server, json, os, pathlib, pty, select, struct, subprocess, sys, tempfile, termios, threading, time

pi = pathlib.Path(sys.argv[1]).resolve()
scenario = sys.argv[2] if len(sys.argv)>2 else "queue"
requests = []
release = threading.Event()
class Server(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_POST(self):
        requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
        turn = len(requests)
        if scenario.startswith('retry') and turn == 1:
            self.send_response(503); self.end_headers(); self.wfile.write(b'controlled overloaded'); return
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        def chunk(text, finish=None):
            self.wfile.write(('data: '+json.dumps({'choices':[{'index':0,'delta':{'content':text},'finish_reason':finish}]})+'\n\n').encode()); self.wfile.flush()
        chunk('PARTIAL_'+str(turn))
        if turn == 1 and not scenario.startswith('retry') and not release.wait(20): return
        try:
            chunk(' DONE_'+str(turn), 'stop')
            self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError): pass
server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Server)
server.daemon_threads = False
threading.Thread(target=server.serve_forever, daemon=True).start()
with tempfile.TemporaryDirectory(prefix='pi-queues-') as root:
    root = pathlib.Path(root); agent = root/'agent'; agent.mkdir(); sessions = root/'sessions'
    (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular','compaction':{'enabled':False},'retry':{'enabled':scenario.startswith('retry'),'maxRetries':2,'baseDelayMs':30000 if scenario=='retry' else 1000,'provider':{'maxRetries':0}}}))
    (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
    master, slave = pty.openpty(); before = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 100, 0, 0))
    env = {'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
    process = subprocess.Popen(['node',str(pi/'packages/coding-agent/dist/cli.js'),'--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)], stdin=slave, stdout=slave, stderr=slave, env=env, cwd=root)
    output = b''
    def wait_for(predicate):
        global output
        end = time.monotonic()+20
        while not predicate():
            if time.monotonic()>end or process.poll() is not None: raise RuntimeError(f'PTY wait failed ({process.poll()}): {output!r}')
            if select.select([master],[],[],.05)[0]: output += os.read(master,65536)
    try:
        wait_for(lambda: b'\x1b[?2004h' in output)
        time.sleep(.5)
        os.write(master,b'first question\r')
        if scenario.startswith('retry'):
            wait_for(lambda: b'Retrying (1/2)' in output)
            if scenario == 'retry':
                os.write(master,b'\x1b')
                wait_for(lambda: b'Retry cancelled' in output)
            else: wait_for(lambda: b'DONE_2' in output)
            time.sleep(.2)
            os.write(master,b'after question\r')
            wait_for(lambda: ('DONE_'+('2' if scenario=='retry' else '3')).encode() in output)
        else:
            wait_for(lambda: b'PARTIAL_1' in output)
            follow = '/quit' if scenario=='commands' else 'follow question'
            steer = '/new' if scenario=='commands' else 'steer question'
            os.write(master,follow.encode()+b'\x1b\r')
            wait_for(lambda: ('Follow-up: '+follow).encode() in output)
            os.write(master,steer.encode()+(b'\x1b\r' if scenario=='commands' else b'\r'))
            wait_for(lambda: (('Follow-up: ' if scenario=='commands' else 'Steering: ')+steer).encode() in output)
            if scenario in ['restore','abort']:
                os.write(master,b'draft')
                time.sleep(.1)
                os.write(master,b'\x1b[1;3A' if scenario=='restore' else b'\x1b[27u')
                time.sleep(.3)
                if scenario=='restore':
                    os.write(master,b'\x03')
                    time.sleep(.1)
                    release.set()
                    wait_for(lambda: b'DONE_1' in output)
                    time.sleep(.2)
                    os.write(master,b'after question\r')
                else:
                    os.write(master,b' edited\r')
                wait_for(lambda: b'DONE_2' in output)
            else:
                release.set()
                wait_for(lambda: b'DONE_3' in output)
                time.sleep(.15)
                os.write(master,b'after question\r')
                wait_for(lambda: b'DONE_4' in output)
        time.sleep(.2)
        os.write(master,b'\x04')
        wait_for(lambda: process.poll() is not None)
        while select.select([master],[],[],.05)[0]: output += os.read(master,65536)
        def text(m):
            c=m['content']; return c if isinstance(c,str) else ''.join(b.get('text','') for b in c)
        prompts = [[text(m) for m in r['messages'] if m['role']=='user'] for r in requests]
        roles=[]; stops=[]; assistant_text=[]
        for path in sessions.rglob('*.jsonl'):
            for line in path.read_text().splitlines():
                row=json.loads(line)
                if row['type']=='message':
                    m=row['message']; roles.append(m['role'])
                    if m['role']=='assistant': stops.append(m['stopReason']); assistant_text.append(text(m))
        print(json.dumps({'prompts':prompts,'roles':roles,'stops':stops,'assistant_text':assistant_text,'retry_visible':b'Retrying (1/2)' in output,'retry_cancelled':b'Retry cancelled' in output,'steering_visible':b'Steering: steer question' in output,'follow_up_visible':b'Follow-up: follow question' in output,'exit_code':process.returncode,'terminal_restored':termios.tcgetattr(slave)==before}))
    finally:
        release.set()
        if process.poll() is None: process.kill(); process.wait()
        os.close(master); os.close(slave)
server.shutdown(); server.server_close()
