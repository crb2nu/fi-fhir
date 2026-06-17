/**
 * Validation for the AutorouteResolver's "Resolve Mapping" form.
 *
 * Resolve and Suggest both require a source code, source system, and target
 * system before a lookup can run. This pure helper returns an inline-ready
 * message describing the missing precondition so the panel can surface it next
 * to the form (the existing error Panel) instead of a transient toast — per the
 * toast-budget policy (.loom/22, B1: persistent validation belongs inline, not
 * in a 4-second toast).
 */

export interface ResolveInputs {
  sourceCode: string;
  sourceSystem: string;
  targetSystem: string;
}

/**
 * Returns an inline-ready message when a required field is missing, otherwise
 * null. The message is identical for any missing field — the `required` markers
 * on the inputs already point the user at which one — matching the single
 * combined guard the component used before this redirect.
 */
export function validateResolveInputs(inputs: ResolveInputs): string | null {
  if (!inputs.sourceCode || !inputs.sourceSystem || !inputs.targetSystem) {
    return 'Source code, source system, and target system are required';
  }
  return null;
}
