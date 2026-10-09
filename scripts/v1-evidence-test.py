#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('v1_live', Path(__file__).with_name('v1-live.py'))
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


class LiveEvidenceTest(unittest.TestCase):
    def test_missing_skipped_failed_and_partial_results_are_rejected(self):
        test = 'TestRequired'
        for events in [[], [{'Action': 'pass', 'Test': 'TestOther'}], [{'Action': 'run', 'Test': test}], [{'Action': 'run', 'Test': test}, {'Action': 'skip', 'Test': test}], [{'Action': 'run', 'Test': test}, {'Action': 'pass', 'Test': test}, {'Action': 'skip', 'Test': test + '/vision'}], [{'Action': 'run', 'Test': test}, {'Action': 'pass', 'Test': test}, {'Action': 'fail'}]]:
            with self.subTest(events=events), self.assertRaises(ValueError):
                live.verify_events(events, test)
        live.verify_events([{'Action': 'run', 'Test': test}, {'Action': 'pass', 'Test': test}], test)


if __name__ == '__main__':
    unittest.main()
