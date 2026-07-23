#!/usr/bin/env node

'use strict';

const { spawnSync } = require('node:child_process');
const path = require('node:path');

const supportedTargets = new Set([
  'darwin-arm64',
  'darwin-x64',
  'linux-arm64',
  'linux-x64',
  'win32-arm64',
  'win32-x64',
]);

function resolveBinary(platform = process.platform, arch = process.arch) {
  const target = `${platform}-${arch}`;
  if (!supportedTargets.has(target)) {
    throw new Error(`Unsupported platform: ${target}`);
  }
  const executable = platform === 'win32' ? 'xapi-conformance.exe' : 'xapi-conformance';
  return path.join(__dirname, '..', 'bin', target, executable);
}

function main() {
  let binary;
  try {
    binary = process.env.XAPI_CONFORMANCE_BINARY || resolveBinary();
  } catch (error) {
    console.error(error.message);
    process.exit(1);
  }

  const args = process.argv.slice(2);
  const hasVectorsFlag = args.some((arg) => arg === '-vectors' || arg === '--vectors' || arg.startsWith('-vectors=') || arg.startsWith('--vectors='));
  const runnerArgs = hasVectorsFlag
    ? args
    : ['-vectors', path.join(__dirname, '..', 'vectors'), ...args];

  const result = spawnSync(binary, runnerArgs, {
    stdio: 'inherit',
    windowsHide: true,
  });

  if (result.error) {
    console.error(`Unable to start the bundled xapi-conformance binary: ${result.error.message}`);
    process.exit(1);
  }

  process.exit(result.status ?? 1);
}

if (require.main === module) {
  main();
}

module.exports = { resolveBinary };
