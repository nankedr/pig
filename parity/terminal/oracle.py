import signal
import fcntl, http.server, json, os, pathlib, pty, re, select, struct, subprocess, sys, tempfile, termios, threading, time

pi = pathlib.Path(sys.argv[1]).resolve()
requests = []
release = threading.Event()
class Server(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def do_POST(self):
        requests.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        def chunk(text, finish=None):
            self.wfile.write(('data: '+json.dumps({'choices':[{'index':0,'delta':{'content':text},'finish_reason':finish}]})+'\n\n').encode()); self.wfile.flush()
        if len(requests) == 1:
            chunk('FIRST_STREAM_TOKEN')
            if not release.wait(15): return
            chunk(' FIRST_DONE', 'stop')
        else: chunk('SECOND_DONE', 'stop')
        self.wfile.write(b'data: [DONE]\n\n'); self.wfile.flush()
server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Server)
threading.Thread(target=server.serve_forever, daemon=True).start()
with tempfile.TemporaryDirectory(prefix='pi-interactive-') as root:
    root = pathlib.Path(root); agent = root/'agent'; agent.mkdir(); sessions = root/'sessions'
    (agent/'settings.json').write_text(json.dumps({'quietStartup':True,'tuiMode':'regular'}))
    (agent/'models.json').write_text(json.dumps({'providers':{'deepseek':{'baseUrl':f'http://127.0.0.1:{server.server_port}','api':'openai-completions','models':[{'id':'deepseek-v4-flash','name':'controlled','contextWindow':65536,'maxTokens':4096,'reasoning':False,'input':['text'],'cost':{'input':0,'output':0,'cacheRead':0,'cacheWrite':0}}]}}}))
    master, slave = pty.openpty(); before = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 100, 0, 0))
    env = {'PATH':os.environ['PATH'],'HOME':str(root),'TERM':'xterm-256color','PI_CODING_AGENT_DIR':str(agent),'PI_OFFLINE':'1','PI_SKIP_VERSION_CHECK':'1'}
    cmd = ['node',str(pi/'packages/coding-agent/dist/cli.js'),'--provider','deepseek','--model','deepseek-v4-flash','--api-key','synthetic','--no-tools','--no-extensions','--no-skills','--session-dir',str(sessions)]
    process = subprocess.Popen(cmd, stdin=slave, stdout=slave, stderr=slave, env=env, cwd=root)
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
        wait_for(lambda: b'FIRST_STREAM_TOKEN' in output)
        streaming = not release.is_set()
        release.set()
        wait_for(lambda: b'FIRST_DONE' in output)
        os.write(master,b'second question\r')
        wait_for(lambda: b'SECOND_DONE' in output)
        time.sleep(.15)
        action = sys.argv[2] if len(sys.argv)>2 else 'ctrl-d'
        if action.startswith('SIG'): process.send_signal(getattr(signal,action))
        else: os.write(master,{'ctrl-d':b'\x04','ctrl-c-twice':b'\x03\x03','quit':b'/quit\r'}[action])
        wait_for(lambda: process.poll() is not None)
        while select.select([master],[],[],.05)[0]: output += os.read(master,65536)
        messages = []
        for path in sessions.rglob('*.jsonl'):
            for line in path.read_text().splitlines():
                row = json.loads(line)
                if row['type']=='message': messages.append(row['message']['role'])
        print(json.dumps({'exit_code':process.returncode,'stream_before_completion':streaming,'first_reply_visible':b'FIRST_DONE' in output,'second_reply_visible':b'SECOND_DONE' in output,'request_count':len(requests),'session_roles':messages,'terminal_restored':termios.tcgetattr(slave)==before,'cursor_restored':output.rfind(b'\x1b[?25h') > output.rfind(b'\x1b[?25l'),'paste_disabled':output.rfind(b'\x1b[?2004l') > output.rfind(b'\x1b[?2004h')}))
    finally:
        release.set()
        if process.poll() is None: process.kill(); process.wait()
        os.close(master); os.close(slave)
server.shutdown()
