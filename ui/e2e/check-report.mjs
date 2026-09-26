// Existence guard for the browser smoke gate: every check, both negative
// controls and every visual capture must have RUN and PASSED in their own
// project. Playwright alone would pass a run in which a project matched no
// spec file, or a renamed test silently stopped existing — exactly how a gate
// gets greener as it gets weaker. Titles are matched by their leading check
// number; visual.spec.ts numbers its captures V1..V11.
import { readFileSync } from 'node:fs';

const required = {
  'operator-bundle': ['1.', '2.', '3.', '4.', '5.'],
  'missing-operator-role': ['6a.'],
  'sessions-off': ['6b.'],
  visual: ['V1.', 'V2.', 'V3.', 'V4.', 'V5.', 'V6.', 'V7.', 'V8.', 'V9.', 'V10.', 'V11.']
};

const [reportPath] = process.argv.slice(2);
if (!reportPath) {
  console.error('usage: check-report.mjs <report.json>');
  process.exit(2);
}
const report = JSON.parse(readFileSync(reportPath, 'utf8'));

const passed = new Map();
const failures = [];
function walk(suite) {
  for (const spec of suite.specs ?? []) {
    for (const run of spec.tests ?? []) {
      if (run.status === 'expected') {
        const titles = passed.get(run.projectName) ?? [];
        titles.push(spec.title);
        passed.set(run.projectName, titles);
      } else {
        failures.push(`${run.projectName} › ${spec.title}: ${run.status}`);
      }
    }
  }
  for (const child of suite.suites ?? []) walk(child);
}
for (const suite of report.suites ?? []) walk(suite);

const missing = [];
for (const [project, prefixes] of Object.entries(required)) {
  const titles = passed.get(project) ?? [];
  for (const prefix of prefixes) {
    if (!titles.some((title) => title.startsWith(`${prefix} `))) {
      missing.push(`${project} › "${prefix} …" did not run and pass`);
    }
  }
}

if (failures.length > 0 || missing.length > 0) {
  for (const line of [...failures, ...missing]) console.error(`[ui-e2e] ${line}`);
  process.exit(1);
}
const total = [...passed.values()].reduce((sum, titles) => sum + titles.length, 0);
process.stdout.write(`[ui-e2e] existence guard: ${total} checks passed across ${passed.size} projects\n`);
