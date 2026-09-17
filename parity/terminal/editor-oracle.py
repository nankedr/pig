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
        chunk('EDITOR_DONE_'+str(len(requests)), 'stop')
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
        cases = json.loads(pathlib.Path(__file__).with_name('editor-cli-cases.json').read_text())
        os.write(master,b'\r   \rdiscard\x03')
        time.sleep(.1)
        for index, turn in enumerate(cases['turns']):
            chunks = turn['chunks']
            for chunk_index, text in enumerate(chunks):
                os.write(master,text.encode())
                time.sleep(.04)
                if chunk_index == 0 and 'columns' in turn:
                    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, turn['columns'], 0, 0))
                    process.send_signal(signal.SIGWINCH)
            wait_for(lambda: ('EDITOR_DONE_'+str(index+1)).encode() in output)
            time.sleep(.1)
        os.write(master,b'\x04')
        wait_for(lambda: process.poll() is not None)
        prompts=[]
        for request in requests:
            content=[m['content'] for m in request['messages'] if m['role']=='user'][-1]
            if isinstance(content,list): content=''.join(c.get('text','') for c in content)
            prompts.append(content)
        print(json.dumps({'prompts':prompts,'exit_code':process.returncode}))
    finally:
        release.set()
        if process.poll() is None: process.kill(); process.wait()
        os.close(master); os.close(slave)
server.shutdown()
