#!/usr/bin/env python3
"""Require fresh protected service and native image results; reject skips."""
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
CASES = [
    ('chat-tools', './codingagent', 'TestDeepSeekLiveHeadlessReadContinuation', 'PIG_REQUIRE_LIVE'),
    ('responses-text', './cmd/pig', 'TestIssue131DeepSeekResponsesLiveText', 'PIG_REQUIRE_RESPONSES_LIVE'),
    ('responses-tools', './agent', 'TestResponsesAgentDeepSeekLiveToolContinuation', 'PIG_REQUIRE_RESPONSES_TOOLS_LIVE'),
    ('responses-restore', './codingagent', 'TestResponsesCodingAgentLiveRestore', 'PIG_REQUIRE_RESPONSES_SESSION_LIVE'),
    ('user-vision-restore', './codingagent', 'TestUserImagesDeepSeekLiveRestore', 'PIG_REQUIRE_VISION_LIVE'),
    ('tool-vision', './codingagent', 'TestToolImagesDeepSeekLiveReadAndWrite', 'PIG_REQUIRE_TOOL_VISION_LIVE'),
    ('rpc-vision', './cmd/pig', 'TestImageWorkflowRPCLive136', 'PIG_REQUIRE_IMAGE_WORKFLOW_LIVE'),
    ('native-clipboard', './codingagent', 'TestImageWorkflowNativeClipboard136', 'PIG_REQUIRE_CLIPBOARD_IMAGE_NATIVE'),
]


def verify_events(events, test):
    actions = [e.get('Action') for e in events if e.get('Test') == test]
    if 'run' not in actions or 'pass' not in actions or any(e.get('Action') in ('skip', 'fail') for e in events):
        raise ValueError('required smoke did not pass without skips: ' + test)


def main():
    if platform.system() != 'Darwin' or platform.machine() != 'arm64':
        raise SystemExit('V1 native smoke requires darwin/arm64')
    if not os.environ.get('DEEPSEEK_API_KEY', '').strip():
        raise SystemExit('V1 freeze/release requires DEEPSEEK_API_KEY; offline tests are not live evidence')
    dest = Path(os.environ.get('PIG_V1_EVIDENCE_DIR') or tempfile.mkdtemp(prefix='pig-v1-live-')).resolve()
    dest.mkdir(parents=True, exist_ok=True)
    record = {'commit': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(), 'platform': 'darwin/arm64', 'started_at': datetime.now(timezone.utc).isoformat(), 'endpoint': 'https://api.deepseek.com', 'official_docs_checked_at': json.loads((ROOT / 'parity/responses-matrix.json').read_text())['checked_at'], 'text_model': 'deepseek-v4-pro', 'vision_model': 'deepseek-flash', 'cases': [], 'status': 'failed'}
    try:
        for name, package, test, flag in CASES:
            args = ['go', 'test', package, '-run', '^' + test + '$', '-count=1', '-json']
            print('RUN', name, flag + '=1', ' '.join(args), flush=True)
            env = {**os.environ, flag: '1', 'PIG_RESPONSES_SMOKE_MODEL': 'deepseek-v4-pro'}
            log = dest / (name + '.jsonl')
            with log.open('w') as stream:
                result = subprocess.run(args, cwd=ROOT, env=env, stdout=stream, stderr=subprocess.STDOUT)
            case = {'id': name, 'command': flag + '=1 ' + ' '.join(args), 'exit_code': result.returncode, 'log': log.name, 'sha256': hashlib.sha256(log.read_bytes()).hexdigest(), 'status': 'failed', 'api': ['openai-responses'], 'model': 'deepseek-v4-pro', 'thinking': 'off (CLI text) / default (SDK)'}
            if name == 'chat-tools':
                case.update(api=['openai-completions'], model='deepseek-v4-flash', thinking='default')
            elif name in ('user-vision-restore', 'tool-vision', 'rpc-vision'):
                case.update(api=['openai-responses'] if name == 'user-vision-restore' else ['openai-responses', 'openai-completions'], model='deepseek-flash', thinking='off')
            elif name == 'responses-restore':
                case['thinking'] = 'low'
            elif name == 'responses-tools':
                case['thinking'] = 'default; max_tokens=4096'
            elif name == 'native-clipboard':
                case.update(api=[], model=None, thinking=None)
            record['cases'].append(case)
            if result.returncode:
                raise ValueError('required smoke failed: ' + name + '; see ' + str(log))
            events = [json.loads(line) for line in log.read_text().splitlines() if line.startswith('{')]
            verify_events(events, test)
            case['status'] = 'passed'
            print('PASS', name, flush=True)
        record['status'] = 'passed'
    finally:
        record['finished_at'] = datetime.now(timezone.utc).isoformat()
        (dest / 'live.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
    print('PASS protected and native smoke:', dest / 'live.json')


if __name__ == '__main__':
    main()
