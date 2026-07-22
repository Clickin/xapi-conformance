#!/usr/bin/env node

const { spawnSync } = require('node:child_process');
const path = require('node:path');
const packageJson = require('../package.json');

const modulePath = process.env.XAPI_CONFORMANCE_GO_MODULE
  || `github.com/Clickin/xapi-conformance@v${packageJson.version}`;
const args = process.argv.slice(2);
const hasVectorsFlag = args.some((arg) => arg === '-vectors' || arg === '--vectors' || arg.startsWith('-vectors=') || arg.startsWith('--vectors='));
const runnerArgs = hasVectorsFlag
  ? args
  : ['-vectors', path.join(__dirname, '..', 'vectors'), ...args];

const result = spawnSync('go', ['run', modulePath, ...runnerArgs], {
  stdio: 'inherit',
  windowsHide: true,
});

if (result.error) {
  if (result.error.code === 'ENOENT') {
    console.error('xapi-conformance requires Go 1.22 or newer. Install Go and try again.');
  } else {
    console.error(`Unable to start Go: ${result.error.message}`);
  }
  process.exit(1);
}

process.exit(result.status ?? 1);
