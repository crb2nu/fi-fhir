#!/usr/bin/env node
'use strict';

/* eslint-disable @typescript-eslint/no-require-imports */

const os = require('os');

// graphql-codegen CLI uses os.cpus().length to set its internal task concurrency.
// In some sandboxed environments os.cpus() can be empty, which causes the CLI to
// throw before generating anything. When that happens, shim a single CPU.
if (os.cpus().length === 0) {
  os.cpus = () => [
    {
      model: 'sandbox',
      speed: 0,
      times: { user: 0, nice: 0, sys: 0, idle: 0, irq: 0 }
    }
  ];
}

// Use a relative path to bypass package "exports" restrictions.
require('../node_modules/@graphql-codegen/cli/cjs/bin.js');
