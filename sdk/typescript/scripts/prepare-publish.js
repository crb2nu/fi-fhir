// Updates sdk/typescript/package.json for publishing.
// - Converts local optionalDependencies (file:packages/...) to the given version string.
// This keeps main branch dev workflow working without requiring published packages.

const fs = require('fs');
const path = require('path');

function usage() {
  // eslint-disable-next-line no-console
  console.error('Usage: node scripts/prepare-publish.js --version <semver>');
  process.exit(2);
}

function parseArgs(argv) {
  const args = { version: '' };
  for (let i = 2; i < argv.length; i++) {
    const a = argv[i];
    if (a === '--version') {
      args.version = argv[i + 1] || '';
      i++;
      continue;
    }
  }
  return args;
}

const { version } = parseArgs(process.argv);
if (!version) usage();

const pkgPath = path.join(__dirname, '..', 'package.json');
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));

if (!pkg.optionalDependencies) {
  pkg.optionalDependencies = {};
}

for (const [name, spec] of Object.entries(pkg.optionalDependencies)) {
  if (!name.startsWith('@fi-fhir/fi-fhir-')) continue;
  if (typeof spec !== 'string') continue;
  if (!spec.startsWith('file:')) continue;
  pkg.optionalDependencies[name] = version;
}

fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');

// eslint-disable-next-line no-console
console.log(`Updated optionalDependencies for publish (version=${version})`);

