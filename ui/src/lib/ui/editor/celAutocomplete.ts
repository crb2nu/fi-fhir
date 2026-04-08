import {
  autocompletion,
  CompletionContext,
  type CompletionResult,
  type Completion
} from '@codemirror/autocomplete';

const CEL_KEYWORDS: Completion[] = [
  { label: 'true', type: 'keyword' },
  { label: 'false', type: 'keyword' },
  { label: 'null', type: 'keyword' },
  { label: 'in', type: 'keyword' },
  { label: 'has', type: 'function', info: 'Check if a field is present' },
  { label: 'size', type: 'function', info: 'Get the length of a string, list, or map' },
  { label: 'matches', type: 'function', info: 'Test if a string matches a regex' },
  { label: 'exists', type: 'function' },
  { label: 'all', type: 'function' },
  { label: 'map', type: 'function' },
  { label: 'filter', type: 'function' },
  { label: 'exists_one', type: 'function' },
  { label: 'type', type: 'function', info: 'Get the type of a value' },
  { label: 'duration', type: 'function', info: 'Create a duration from a string' },
  { label: 'timestamp', type: 'function', info: 'Create a timestamp from a string' }
];

const CEL_VARIABLES: Completion[] = [
  { label: 'event', type: 'variable', info: 'The current semantic event' },
  { label: 'event.id', type: 'property', info: 'Unique ID of the event' },
  { label: 'event.type', type: 'property', info: 'Type of the event (e.g. PATIENT_ADMIT)' },
  { label: 'event.source', type: 'property', info: 'Origin feed of the event' },
  { label: 'event.timestamp', type: 'property', info: 'When the event occurred' },
  { label: 'event.payload', type: 'property', info: 'The raw message payload' },
  { label: 'context', type: 'variable', info: 'Workflow execution context' }
];

function celCompletions(context: CompletionContext): CompletionResult | null {
  const word = context.matchBefore(/\w*\.?\w*/);
  if (!word || (word.from === word.to && !context.explicit)) {
    return null;
  }

  // Determine context
  const text = word.text;
  let options = CEL_KEYWORDS.concat(CEL_VARIABLES);

  if (text.startsWith('event.')) {
    options = CEL_VARIABLES.filter(c => c.label.startsWith('event.'));
  }

  return {
    from: word.from,
    options,
    validFor: /^\w*\.?\w*$/
  };
}

export function celAutocomplete() {
  return autocompletion({
    override: [celCompletions]
  });
}
