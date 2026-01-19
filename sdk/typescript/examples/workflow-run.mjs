import { readFileSync } from 'fs';
import { Workflow } from '../dist/index.mjs';

const workflowPath = process.argv[2];
const eventsPath = process.argv[3];

if (!workflowPath || !eventsPath) {
  // eslint-disable-next-line no-console
  console.error('Usage: node examples/workflow-run.mjs <workflow.yaml> <events.json>');
  process.exit(2);
}

const events = JSON.parse(readFileSync(eventsPath, 'utf8'));
const workflow = new Workflow(workflowPath);

const validation = await workflow.validate();
if (!validation.valid) {
  // eslint-disable-next-line no-console
  console.error('Workflow validation failed:', validation.errors);
  process.exit(1);
}

const result = await workflow.run(events);
// eslint-disable-next-line no-console
console.log(JSON.stringify(result, null, 2));

