#!/usr/bin/env python3
"""Bind the V1 branches to existing Catalog contracts and current evidence."""
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]


def audit():
    scope = json.loads((ROOT / 'parity/delivery-scope.json').read_text())
    entries = {e['id']: e for e in map(json.loads, (ROOT / 'parity/catalog.jsonl').read_text().splitlines())}
    acceptance = json.loads((ROOT / 'parity/v1-acceptance.json').read_text())
    groups = {g['id']: g for g in scope['scopes'] if g['version'] == 'V1'}
    if set(acceptance) != set(groups):
        raise ValueError('V1 acceptance must cover every V1 scope exactly once')
    rows = []
    for name, group in groups.items():
        check = acceptance[name]
        if not check['branch'] or not check['remaining'] or not check['public_boundary']:
            raise ValueError('missing versioned branch boundary: ' + name)
        for id in group['catalog_ids']:
            entry = entries[id]
            if entry.get('deferred'):
                raise ValueError('deferred V1 anchor requires an explicit scope decision: ' + id)
            record = {'scope': name, 'branch': check['branch'], 'remaining': check['remaining'], 'entry': entry, 'public_boundary': check['public_boundary']}
            refs = []
            for contract in set([id, *check['contracts']]):
                evidence = entries[contract].get('evidence', [])
                if contract != id and not evidence:
                    raise ValueError('missing boundary evidence: ' + contract)
                for item in evidence:
                    relative, _, fragment = item['ref'].partition('#')
                    path = (ROOT / relative).resolve()
                    if not path.is_relative_to(ROOT) or not path.is_file():
                        raise ValueError('unresolved evidence: ' + item['ref'])
                    data = path.read_bytes()
                    if item['kind'] == 'go-test' and fragment and ('func ' + fragment + '(').encode() not in data:
                        raise ValueError('unresolved test: ' + item['ref'])
                    refs.append(item['ref'] + '=' + hashlib.sha256(data).hexdigest())
            encoded = json.dumps(record, sort_keys=True, separators=(',', ':')).encode()
            rows.append('\t'.join([name, id, entry['status'], hashlib.sha256(encoded).hexdigest(), hashlib.sha256('\n'.join(sorted(set(refs))).encode()).hexdigest()]))
    for name in ['parity/v1-acceptance.json', 'parity/delivery-scope.json', 'parity/responses-matrix.json', 'parity/user-images-matrix.json', 'parity/tool-images-matrix.json', 'parity/image-workflow-matrix.json']:
        rows.append('file\t' + name + '\t' + hashlib.sha256((ROOT / name).read_bytes()).hexdigest())
    snapshot = '\n'.join(sorted(rows)) + '\n'
    path = ROOT / 'internal/v1gate/testdata/catalog_scope.txt'
    if '--write' in sys.argv:
        path.write_text(snapshot)
    elif path.read_text() != snapshot:
        raise ValueError('V1 branch/Catalog/evidence drift; review before running scripts/v1-audit.py --write')
    print('PASS V1 branch audit:', sum(len(g['catalog_ids']) for g in groups.values()), 'anchors; shared partial entries retain V2 gaps; no status promoted')


if __name__ == '__main__':
    audit()
