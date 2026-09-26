import { getContext, setContext } from 'svelte';

/**
 * Field → control wiring. A Field publishes its control id, the id(s) of its
 * hint/error text and its invalid/required state; Input, Select and Textarea
 * read it so `<Field label error><Input /></Field>` is labelled and described
 * without the caller threading ids by hand.
 */
export interface FieldContext {
  readonly id: string;
  readonly describedBy: string | undefined;
  readonly invalid: boolean;
  readonly required: boolean;
}

const FIELD_CONTEXT = Symbol('ui-field');

export function setFieldContext(context: FieldContext): void {
  setContext(FIELD_CONTEXT, context);
}

export function getFieldContext(): FieldContext | undefined {
  return getContext<FieldContext | undefined>(FIELD_CONTEXT);
}

/** Join aria-describedby ids, dropping empties. */
export function joinIds(...ids: Array<string | null | undefined>): string | undefined {
  const joined = ids.filter((id): id is string => Boolean(id)).join(' ');
  return joined || undefined;
}
