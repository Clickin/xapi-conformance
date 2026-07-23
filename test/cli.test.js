'use strict';

const assert = require('node:assert/strict');
const { chmodSync, mkdtempSync, writeFileSync } = require('node:fs');
const { tmpdir } = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const test = require('node:test');
const { resolveBinary } = require('../cli/index.js');

const targets = [
  ['darwin', 'arm64', 'xapi-conformance'],
  ['darwin', 'x64', 'xapi-conformance'],
  ['linux', 'arm64', 'xapi-conformance'],
  ['linux', 'x64', 'xapi-conformance'],
  ['win32', 'arm64', 'xapi-conformance.exe'],
  ['win32', 'x64', 'xapi-conformance.exe'],
];

for (const [platform, arch, executable] of targets) {
  test(`resolves ${platform}-${arch}`, () => {
    assert.equal(
      resolveBinary(platform, arch),
      path.join(__dirname, '..', 'bin', `${platform}-${arch}`, executable),
    );
  });
}

test('rejects an unsupported target', () => {
  assert.throws(() => resolveBinary('freebsd', 'x64'), /Unsupported platform: freebsd-x64/);
});

test('executes a prebuilt binary with the packaged vector directory', () => {
  const dir = mkdtempSync(path.join(tmpdir(), 'xapi-conformance-'));
  const binary = path.join(dir, 'xapi-conformance');
  writeFileSync(binary, '#!/usr/bin/env node\nprocess.stdout.write(JSON.stringify(process.argv.slice(2)));\n');
  chmodSync(binary, 0o755);

  const result = spawnSync(process.execPath, [path.join(__dirname, '..', 'cli', 'index.js'), '-profile', 'nexacro-json-1.0'], {
    encoding: 'utf8',
    env: {...process.env, XAPI_CONFORMANCE_BINARY: binary},
  });

  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), [
    '-vectors',
    path.join(__dirname, '..', 'vectors'),
    '-profile',
    'nexacro-json-1.0',
  ]);
});
