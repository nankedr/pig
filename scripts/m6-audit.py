#!/usr/bin/env python3
"""Lock every M6 mapping and its evidence without promoting capability status."""
import collections
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
BASELINE = '936aff00918de1187f085f123c2812d8f2d67745'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def audit():
    entries = [json.loads(line) for line in (ROOT / 'parity/catalog.jsonl').read_text().splitlines()]
    scope = [e for e in entries if e['milestone'] == 'M6']
    rows = []
    for entry in scope:
        if entry['upstream']['commit'] != BASELINE:
            raise ValueError('wrong baseline: ' + entry['id'])
        evidence = entry.get('evidence', [])
        if entry['status'] == 'partial' and not all(entry.get('partial', {}).get(k) for k in ['supported', 'unsupported']):
            raise ValueError('missing partial boundary: ' + entry['id'])
        hashes = []
        for item in evidence:
            path, _, fragment = item['ref'].partition('#')
            content = (ROOT / path).read_bytes()
            if item['kind'] == 'go-test' and fragment and ('func ' + fragment + '(').encode() not in content:
                raise ValueError('unresolved test: ' + item['ref'])
            if entry['id'] == 'contract:codingagent/m6-workflow' and item['input_hash'] != 'sha256:' + digest(content):
                raise ValueError('M6 workflow evidence hash drift: ' + item['ref'])
            hashes.append(item['ref'] + '=' + digest(content))
        row = {k: entry.get(k) for k in ['id', 'status', 'mapping', 'partial', 'notes', 'evidence']}
        encoded = json.dumps(row, sort_keys=True, separators=(',', ':')).encode()
        rows.append('\t'.join([entry['id'], entry['status'], entry['mapping']['target'], digest(encoded), digest('\n'.join(sorted(hashes)).encode())]))
    # Upstream test inventory and Go API snapshots are separately locked, including slices whose entries belong to earlier milestones.
    paths = ['parity/inventory/manifest.json', 'parity/inventory/files.jsonl', 'parity/surface/symbols.jsonl', 'parity/interactive-settings-inventory.json']
    paths += [str(p.relative_to(ROOT)) for directory in ['codingagent/testdata', 'tui/testdata'] for p in (ROOT / directory).rglob('*') if p.is_file() and ('surface' in p.name or 'api' in p.name or p.suffix == '.txt')]
    for name in sorted(set(paths)):
        rows.append('file\t' + name + '\t' + digest((ROOT / name).read_bytes()))
    snapshot = '\n'.join(sorted(rows)) + '\n'
    target = ROOT / 'internal/m6gate/testdata/catalog_scope.txt'
    if '--write' in sys.argv:
        target.write_text(snapshot)
    elif target.read_text() != snapshot:
        raise ValueError('M6 scope/evidence/API drift; audit changes before running scripts/m6-audit.py --write')
    print('M6 audit:', len(scope), 'entries;', dict(sorted(collections.Counter(e['status'] for e in scope).items())))
    print('inventoried/scaffolded rows and unsupported partial branches are NOT behavioral coverage or frozen APIs')


if __name__ == '__main__':
    audit()
