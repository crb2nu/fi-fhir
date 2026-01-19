import { readFileSync } from 'fs';
import { parseHL7 } from '../dist/index.mjs';

const inputPath = process.argv[2];
if (!inputPath) {
  // eslint-disable-next-line no-console
  console.error('Usage: node examples/parse-hl7.mjs <path-to-hl7-message>');
  process.exit(2);
}

const message = readFileSync(inputPath, 'utf8');
const event = await parseHL7(message, { source: 'example' });
// eslint-disable-next-line no-console
console.log(JSON.stringify(event, null, 2));

