import pathlib
import subprocess
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class Launcher(unittest.TestCase):
    def test_dsm_session_token_required_and_forwarded(self):
        script = r'''
const vm = require('node:vm');
const fs = require('node:fs');
const assert = require('node:assert/strict');
const code = fs.readFileSync(process.argv[1], 'utf8');
function launch(opener) {
  let target = null;
  const status = {textContent: ''};
  vm.runInNewContext(code, {
    window: {opener},
    location: {origin: 'http://nas:5000', replace: url => { target = url; }},
    document: {getElementById: () => status}, URL
  });
  return {target, status};
}
assert.equal(launch(null).target, null);
assert.equal(launch({SYNO: {SDS: {Session: {}}}}).target, null);
const foreign = {};
Object.defineProperty(foreign, 'SYNO', {get() { throw new Error('cross origin'); }});
assert.equal(launch(foreign).target, null);
const result = launch({SYNO: {SDS: {Session: {SynoToken: 'session-token'}}}});
const url = new URL(result.target);
assert.equal(url.origin, 'http://nas:5000');
assert.equal(url.pathname, '/webman/3rdparty/ddnsto/settings.cgi');
assert.equal(url.searchParams.get('SynoToken'), 'session-token');
'''
        subprocess.run(['node', '-e', script, str(ROOT / 'package/web/launch.js')], check=True)
