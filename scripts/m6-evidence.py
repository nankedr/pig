#!/usr/bin/env python3
"""Validate terminal evidence or an explicit, commit-bound release waiver."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
CASES = ['editing-paste', 'keys-legacy', 'keys-kitty', 'keys-modifyOtherKeys', 'render-scroll-resize', 'theme-settings', 'dialogs-selectors', 'queue-cancel', 'bash-abort', 'external-editor', 'maintenance', 'normal-exit', 'signal-recovery', 'editor-failure-recovery']


def validate(record, base, commit):
    if record.get('commit') != commit or record.get('platform') != 'darwin/arm64':
        raise ValueError('manual evidence must identify the exact candidate commit and darwin/arm64')
    if record.get('status') == 'waived':
        if record.get('release') != 'v0.6.0' or record.get('cases') != []:
            raise ValueError('waiver must target v0.6.0 and must not claim tested cases')
        for field in ['approved_by', 'approved_at', 'reason', 'authorization']:
            if not isinstance(record.get(field), str) or not record[field].strip():
                raise ValueError('missing waiver ' + field)
        return 'waived'
    if record.get('status', 'passed') != 'passed':
        raise ValueError('unknown manual acceptance status')
    for field in ['operator', 'tested_at', 'os_version']:
        if not isinstance(record.get(field), str) or not record[field].strip():
            raise ValueError('missing ' + field)
    cases = record.get('cases', [])
    if len(cases) != len(CASES) or {case.get('id') for case in cases} != set(CASES):
        raise ValueError('manual evidence must cover every required case exactly once')
    for case in cases:
        if case.get('result') != 'pass':
            raise ValueError('manual case not passed: ' + case['id'])
        for field in ['terminal', 'terminal_version', 'steps', 'observed']:
            if not isinstance(case.get(field), str) or not case[field].strip():
                raise ValueError('missing ' + field + ': ' + case['id'])
        artifacts = case.get('artifacts', [])
        if not artifacts:
            raise ValueError('missing terminal capture: ' + case['id'])
        for artifact in artifacts:
            relative = Path(artifact['path'])
            if relative.is_absolute() or '..' in relative.parts or relative == Path('evidence.json'):
                raise ValueError('artifact paths must be relative and must not overwrite evidence.json')
            path = (base / relative).resolve()
            if not path.is_relative_to(base.resolve()) or not path.is_file():
                raise ValueError('artifact must be a file inside the evidence directory')
            if hashlib.sha256(path.read_bytes()).hexdigest() != artifact.get('sha256'):
                raise ValueError('terminal capture hash mismatch')
    return 'passed'


def main():
    if sys.argv[1:] == ['--template']:
        print(json.dumps({'commit': '', 'platform': 'darwin/arm64', 'operator': '', 'tested_at': '', 'os_version': '', 'cases': [{'id': case, 'terminal': '', 'terminal_version': '', 'steps': '', 'observed': '', 'result': 'pending', 'artifacts': []} for case in CASES]}, ensure_ascii=False, indent=2))
        return
    if len(sys.argv) != 2 or not sys.argv[1]:
        raise ValueError('PIG_M6_MANUAL_EVIDENCE must name completed human terminal evidence; see docs/learning/m6-manual-acceptance.md')
    path = Path(sys.argv[1]).resolve()
    commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    status = validate(json.loads(path.read_text()), path.parent, commit)
    if status == 'waived':
        print('WAIVED: M6 human terminal acceptance was NOT executed; explicit v0.6.0 approval for ' + commit)
    else:
        print('PASS: M6 human terminal evidence for ' + commit)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError) as error:
        sys.exit(str(error))
