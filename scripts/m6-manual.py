#!/usr/bin/env python3
"""Launch the installed CLI in a real terminal against a synthetic loopback model."""
import argparse
import http.server
import json
import os
from pathlib import Path
import subprocess
import threading
import time

parser = argparse.ArgumentParser()
parser.add_argument('binary', type=Path)
parser.add_argument('directory', type=Path)
parser.add_argument('--fullscreen', action='store_true')
args = parser.parse_args()
root = args.directory.resolve()
root.mkdir(parents=True, exist_ok=True)
if any(root.iterdir()):
    parser.error('directory must be empty; captures and sessions are retained here')
agent = root / 'agent'
agent.mkdir()
(agent / 'settings.json').write_text(json.dumps({'quietStartup': True, 'tuiMode': 'fullscreen' if args.fullscreen else 'regular', 'compaction': {'enabled': False, 'reserveTokens': 1000, 'keepRecentTokens': 10}, 'retry': {'enabled': False}}))


class Handler(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        request = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
        summary = 'context summarization assistant' in json.dumps(request['messages'][0])
        self.send_response(200)
        self.send_header('Content-Type', 'text/event-stream')
        self.end_headers()
        try:
            lines = ['SUMMARY_MANUAL\n'] if summary else [f'ROW_{i:03d} 中文 🐷 manual text\n' for i in range(80)]
            for text in lines:
                chunk = {'choices': [{'index': 0, 'delta': {'content': text}, 'finish_reason': None}]}
                self.wfile.write(('data: ' + json.dumps(chunk) + '\n\n').encode())
                self.wfile.flush()
                time.sleep(.1)
            self.wfile.write(b'data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n')
            self.wfile.flush()
        except (BrokenPipeError, ConnectionResetError):
            pass


server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
env = {**os.environ, 'PIG_CODING_AGENT_DIR': str(agent), 'PIG_OFFLINE': '1', 'PIG_DEEPSEEK_BASE_URL': f'http://127.0.0.1:{server.server_port}'}
try:
    child = subprocess.Popen([str(args.binary.resolve()), '--offline', '--provider', 'deepseek', '--model', 'deepseek-v4-flash', '--api-key', 'synthetic', '--no-tools', '--no-extensions', '--no-approve', '--session-dir', str(root / 'sessions')], cwd=root, env=env)
    print(f'\nPig PID={child.pid}; retained artifacts={root}', flush=True)
    code = child.wait()
    (root / 'exit.json').write_text(json.dumps({'pid': child.pid, 'exit_code': code}) + '\n')
    raise SystemExit(code)
finally:
    server.shutdown()
    server.server_close()
