import copy
import hashlib
import importlib.util
import tempfile
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('evidence', Path(__file__).with_name('m6-evidence.py'))
evidence = importlib.util.module_from_spec(spec)
spec.loader.exec_module(evidence)


class EvidenceTests(unittest.TestCase):
    def test_waiver_is_explicit_scoped_and_not_a_pass(self):
        record = {'commit': 'candidate', 'platform': 'darwin/arm64', 'status': 'waived', 'release': 'v0.6.0', 'approved_by': 'user', 'approved_at': '2026-09-24', 'reason': 'deferred physical terminal check', 'authorization': 'explicit release approval', 'cases': []}
        self.assertEqual(evidence.validate(record, Path('.'), 'candidate'), 'waived')
        for field, value in [('commit', 'stale'), ('status', 'skip'), ('release', 'v0.7.0'), ('approved_by', ''), ('approved_at', ''), ('reason', ''), ('authorization', ''), ('cases', [{'result': 'pass'}])]:
            with self.assertRaises(ValueError):
                evidence.validate({**record, field: value}, Path('.'), 'candidate')

    def test_rejects_missing_stale_and_forged_records(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            (base / 'capture').write_bytes(b'fixture')
            record = {'commit': 'candidate', 'platform': 'darwin/arm64', 'operator': 'test', 'tested_at': '2026-09-24', 'os_version': 'fixture', 'cases': [{'id': case, 'terminal': 'fixture', 'terminal_version': '1', 'steps': 'fixture steps', 'observed': 'fixture observations', 'result': 'pass', 'artifacts': [{'path': 'capture', 'sha256': hashlib.sha256(b'fixture').hexdigest()}]} for case in evidence.CASES]}
            evidence.validate(record, base, 'candidate')
            for field, value in [('commit', 'stale'), ('platform', 'linux/amd64'), ('operator', ''), ('cases', record['cases'][:-1])]:
                altered = {**record, field: value}
                with self.assertRaises(ValueError): evidence.validate(altered, base, 'candidate')
            for field, value in [('result', 'pending'), ('steps', ''), ('artifacts', [])]:
                altered = copy.deepcopy(record)
                altered['cases'][0][field] = value
                with self.assertRaises(ValueError): evidence.validate(altered, base, 'candidate')
            (base / 'capture').write_bytes(b'changed')
            with self.assertRaises(ValueError): evidence.validate(record, base, 'candidate')


if __name__ == '__main__':
    unittest.main()
