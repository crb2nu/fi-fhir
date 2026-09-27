#!/usr/bin/env node
// wasm-smoke.mjs — load the fi-fhir browser kernel in Node and assert its
// JavaScript contract (cmd/fi-fhir-wasm/README.md).
//
//   node scripts/wasm-smoke.mjs [dir]      # dir holds fi-fhir.wasm + wasm_exec.js
//                                          # (default dist/wasm; `make wasm-smoke`)
//
// Node 20+, no dependencies. It is what flexinfer-site's release fetcher runs
// against a downloaded module before installing it, so it checks only the
// contract a page relies on, not engine internals:
//
//   * fiFhirReady turns true within 10 s and every global is a function;
//   * fiFhirVersion() reports kernel "slim";
//   * fiFhirProfiles() and fiFhirSamples() are non-empty arrays of the
//     documented shape, and every profile validates;
//   * fiFhirPreview() on every sample returns the documented shape, with the
//     outcome pinned per sample (event type, Bundle or bundleProblem code);
//   * bad input of every kind comes back as {ok:false, problems:[...]} and
//     never throws;
//   * nothing touches the network (fetch is a tripwire) or the filesystem
//     (Go's wasm_exec.js answers ENOSYS without globalThis.fs).
//
// It prints a size table (raw and gzip -9) and exits non-zero on any failure.

import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import zlib from 'node:zlib';

const dir = process.argv[2] ?? 'dist/wasm';
const wasmPath = path.join(dir, 'fi-fhir.wasm');
const shimPath = path.join(dir, 'wasm_exec.js');

const GLOBALS = [
  'fiFhirVersion',
  'fiFhirProfiles',
  'fiFhirSamples',
  'fiFhirPreview',
  'fiFhirValidateProfile',
];

// Pinned outcomes for the six IDE demo samples without a profile.
const EXPECTED = {
  'adt-a01-icu-admission': { ok: true, type: 'patient_admit', bundle: true },
  'oru-r01-lab-results-cbc': { ok: true, type: 'lab_result', bundle: true },
  'siu-s12-appointment-scheduled': {
    ok: true,
    type: 'appointment_scheduled',
    bundleProblem: 'FHIR_PROJECTION_UNSUPPORTED',
  },
  'adt-a03-discharge-with-warnings': { ok: true, type: 'patient_discharge', bundle: true },
  'orm-o01-lab-order': { ok: false, problem: 'PARSE_FAILED' },
  'mdm-t02-document-with-content': {
    ok: true,
    type: 'document_original',
    bundleProblem: 'FHIR_PROJECTION_UNSUPPORTED',
  },
};

let failures = 0;
function check(condition, message) {
  if (!condition) {
    failures += 1;
    console.error(`FAIL ${message}`);
  }
}

function call(name, ...args) {
  let raw;
  try {
    raw = globalThis[name](...args);
  } catch (error) {
    check(false, `${name} threw: ${error}`);
    return undefined;
  }
  check(typeof raw === 'string', `${name} returned ${typeof raw}, not a string`);
  try {
    return JSON.parse(raw);
  } catch (error) {
    check(false, `${name} returned invalid JSON: ${error}`);
    return undefined;
  }
}

function isProblemList(value) {
  return (
    Array.isArray(value) &&
    value.every(
      (p) => typeof p.code === 'string' && typeof p.path === 'string' && typeof p.message === 'string'
    )
  );
}

function checkPreviewShape(label, r) {
  if (!r) return;
  check(typeof r.ok === 'boolean', `${label}: ok is not a boolean`);
  check(Array.isArray(r.segments), `${label}: segments is not an array`);
  check(Array.isArray(r.events), `${label}: events is not an array`);
  check(Array.isArray(r.diagnostics), `${label}: diagnostics is not an array`);
  check(isProblemList(r.problems), `${label}: problems is not a problem list`);
  for (const s of r.segments) {
    check(
      Number.isInteger(s.index) && typeof s.id === 'string' && Array.isArray(s.fields),
      `${label}: bad segment ${JSON.stringify(s).slice(0, 80)}`
    );
  }
  for (const e of r.events) {
    check(typeof e.type === 'string' && e.payload && typeof e.payload === 'object', `${label}: bad event`);
  }
  for (const d of r.diagnostics) {
    check(
      ['severity', 'code', 'path', 'message'].every((k) => typeof d[k] === 'string'),
      `${label}: bad diagnostic ${JSON.stringify(d)}`
    );
  }
  if (r.bundleProblem !== undefined) {
    check(isProblemList([r.bundleProblem]), `${label}: bad bundleProblem`);
  }
  check(r.ok === (r.problems.length === 0), `${label}: ok disagrees with problems`);
}

function expectProblem(label, r, code) {
  check(r && r.ok === false, `${label}: ok is not false`);
  check(
    r && isProblemList(r.problems) && r.problems[0]?.code === code,
    `${label}: want problem ${code}, got ${JSON.stringify(r?.problems)}`
  );
}

async function main() {
  const major = Number(process.versions.node.split('.')[0]);
  if (major < 20) throw new Error(`Node 20+ required, running ${process.versions.node}`);

  // Network tripwire: the module must never reach for fetch.
  let networkCalls = 0;
  globalThis.fetch = () => {
    networkCalls += 1;
    throw new Error('wasm-smoke: network access is forbidden');
  };
  // No globalThis.fs: wasm_exec.js then answers every file call with ENOSYS.
  delete globalThis.fs;

  vm.runInThisContext(fs.readFileSync(shimPath, 'utf8'), { filename: shimPath });
  const bytes = fs.readFileSync(wasmPath);
  const go = new globalThis.Go();
  const started = performance.now();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance); // resolves only if main returns; main blocks on select{}

  const deadline = Date.now() + 10_000;
  while (globalThis.fiFhirReady !== true) {
    if (Date.now() > deadline) throw new Error('fiFhirReady did not turn true within 10 s');
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  const readyMs = performance.now() - started;
  for (const name of GLOBALS) {
    check(typeof globalThis[name] === 'function', `${name} is not a function`);
  }
  if (failures) throw new Error('globals missing');

  const version = call('fiFhirVersion');
  check(version?.kernel === 'slim', `fiFhirVersion kernel = ${version?.kernel}`);
  for (const key of ['version', 'commit', 'builtAt']) {
    check(typeof version?.[key] === 'string', `fiFhirVersion.${key} is not a string`);
  }

  const profiles = call('fiFhirProfiles') ?? [];
  check(Array.isArray(profiles) && profiles.length > 0, 'fiFhirProfiles is empty');
  for (const p of profiles) {
    check(
      ['id', 'name', 'description', 'yaml'].every((k) => typeof p[k] === 'string' && p[k] !== ''),
      `profile shape ${JSON.stringify(p).slice(0, 80)}`
    );
    const v = call('fiFhirValidateProfile', p.yaml);
    check(v?.ok === true && isProblemList(v?.problems) && v.problems.length === 0, `profile ${p.id} does not validate: ${JSON.stringify(v?.problems)}`);
  }

  const samples = call('fiFhirSamples') ?? [];
  check(Array.isArray(samples) && samples.length === 6, `fiFhirSamples has ${samples.length} samples, want 6`);
  const rows = [];
  for (const s of samples) {
    check(
      ['id', 'name', 'source', 'format', 'text'].every((k) => typeof s[k] === 'string' && s[k] !== ''),
      `sample shape ${JSON.stringify(s).slice(0, 80)}`
    );
    const t0 = performance.now();
    const r = call('fiFhirPreview', JSON.stringify({ message: s.text, format: s.format, source: s.source }));
    const ms = performance.now() - t0;
    checkPreviewShape(`preview ${s.id}`, r);
    const want = EXPECTED[s.id];
    check(want !== undefined, `no pinned outcome for sample ${s.id}`);
    if (r && want) {
      check(r.ok === want.ok, `preview ${s.id}: ok = ${r.ok}, want ${want.ok} (${JSON.stringify(r.problems)})`);
      check(r.segments[0]?.id === 'MSH', `preview ${s.id}: first segment is not MSH`);
      if (want.ok) {
        check(r.events.length === 1 && r.events[0].type === want.type, `preview ${s.id}: event ${r.events[0]?.type}, want ${want.type}`);
      } else {
        check(r.problems[0]?.code === want.problem, `preview ${s.id}: problem ${r.problems[0]?.code}, want ${want.problem}`);
      }
      if (want.bundle) {
        check(
          r.bundle?.resourceType === 'Bundle' && r.bundle?.type === 'transaction' && r.bundle.entry?.length > 0,
          `preview ${s.id}: no transaction Bundle`
        );
      }
      if (want.bundleProblem) {
        check(r.bundle === undefined && r.bundleProblem?.code === want.bundleProblem, `preview ${s.id}: bundleProblem ${r.bundleProblem?.code}, want ${want.bundleProblem}`);
      }
    }
    rows.push({
      sample: s.id,
      ok: r?.ok,
      event: r?.events?.[0]?.type ?? '-',
      diagnostics: r?.diagnostics?.length,
      bundleEntries: r?.bundle?.entry?.length ?? 0,
      ms: Number(ms.toFixed(1)),
    });
  }

  // Every built-in profile against the ADT sample.
  const adt = samples.find((s) => s.id === 'adt-a01-icu-admission');
  for (const p of profiles) {
    if (!adt) break;
    const r = call('fiFhirPreview', JSON.stringify({ message: adt.text, format: 'hl7v2', profileYaml: p.yaml }));
    checkPreviewShape(`preview adt with ${p.id}`, r);
    check(r?.ok === true, `preview adt with ${p.id}: ${JSON.stringify(r?.problems)}`);
  }

  // Bad input never throws and always names the problem.
  expectProblem('no argument', call('fiFhirPreview'), 'INPUT_INVALID');
  expectProblem('number argument', call('fiFhirPreview', 42), 'INPUT_INVALID');
  expectProblem('not JSON', call('fiFhirPreview', 'MSH|^~\\&|'), 'INPUT_INVALID');
  expectProblem('unknown field', call('fiFhirPreview', JSON.stringify({ message: 'MSH|', format: 'hl7v2', extra: 1 })), 'INPUT_INVALID');
  expectProblem('empty message', call('fiFhirPreview', JSON.stringify({ message: '', format: 'hl7v2' })), 'MESSAGE_EMPTY');
  expectProblem(
    'message over 1 MiB',
    call('fiFhirPreview', JSON.stringify({ message: 'MSH|' + 'x'.repeat(1 << 20), format: 'hl7v2' })),
    'MESSAGE_TOO_LARGE'
  );
  expectProblem('wrong format', call('fiFhirPreview', JSON.stringify({ message: 'MSH|', format: 'x12' })), 'FORMAT_UNSUPPORTED');
  expectProblem('bad profile', call('fiFhirPreview', JSON.stringify({ message: adt?.text ?? 'MSH|', format: 'hl7v2', profileYaml: 'hl7v2: [' })), 'PROFILE_SYNTAX');
  expectProblem('bad timezone', call('fiFhirPreview', JSON.stringify({ message: adt?.text ?? 'MSH|', format: 'hl7v2', timezone: 'Nowhere/Special' })), 'TIMEZONE_INVALID');
  expectProblem('not HL7', call('fiFhirPreview', JSON.stringify({ message: 'hello', format: 'hl7v2' })), 'PARSE_FAILED');
  expectProblem('empty profile', call('fiFhirValidateProfile', ''), 'PROFILE_EMPTY');
  expectProblem('profile argument missing', call('fiFhirValidateProfile'), 'INPUT_INVALID');
  expectProblem('unsupported profile', call('fiFhirValidateProfile', 'hl7v2:\n  default_version: "2.1"\n  timezone: UTC\n  event_classifications:\n    - {message_type: ADT^A01, event_type: patient_admit, priority: 1}\n'), 'PROFILE_UNSUPPORTED');

  check(networkCalls === 0, `the module called fetch ${networkCalls} times`);

  const raw = bytes.length;
  const gzip = zlib.gzipSync(bytes, { level: 9 }).length;
  console.log(`fi-fhir wasm ${version?.version} (${version?.commit?.slice(0, 12) || 'no commit'}), ready in ${readyMs.toFixed(0)} ms`);
  console.table(rows);
  console.log(`| file | raw bytes | gzip -9 bytes |`);
  console.log(`|---|---|---|`);
  console.log(`| fi-fhir.wasm | ${raw} | ${gzip} |`);
  console.log(`| wasm_exec.js | ${fs.statSync(shimPath).size} | - |`);
  if (failures) throw new Error(`${failures} check(s) failed`);
  console.log('wasm-smoke: ok');
}

main().then(
  () => process.exit(0),
  (error) => {
    console.error(`wasm-smoke: ${error.message}`);
    process.exit(1);
  }
);
