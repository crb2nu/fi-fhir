import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render, screen, within } from '@testing-library/svelte';
import ConnectionForm from './ConnectionForm.svelte';
import { newBuffer, type EditBuffer } from './editBuffer';
import { reactive } from './testState.svelte';
import {
  BINDING_DECLARE_FIRST,
  KIND_SCHEMAS,
  SFTP_CREDENTIAL,
  buildSpec,
  controlValue,
  emptyRepeatItem,
  endpointOf,
  groupFields,
  placeProblems,
  repeatSections,
  valuesFromSpec,
  type FieldCondition,
  type KindSchema,
  type SpecField,
  type SpecProblem
} from './specSchema';

/**
 * C-0's own worked examples (internal/integration/connection/testdata), the
 * documents its checker and constructor tests compile. The form model must
 * carry every key they use, unchanged.
 */
const FIXTURES = import.meta.glob<{ kind: string; spec: Record<string, unknown> }>(
  '../../../../../internal/integration/connection/testdata/*.json',
  { eager: true, import: 'default' }
);

const noIdentityErrors = { id: null, name: null, description: null };

function renderForm(
  schema: KindSchema,
  configure: (buffer: EditBuffer) => void = () => {},
  problems: SpecProblem[] = []
) {
  const buffer = reactive(newBuffer(schema.kind));
  configure(buffer);
  const placed = placeProblems(
    schema,
    problems,
    buffer.values,
    buffer.bindings.map((binding) => binding.name)
  );
  return render(ConnectionForm, {
    props: { schema, buffer, placed, identityErrors: noIdentityErrors }
  });
}

/** A value that satisfies a field's condition. */
function satisfying(condition: FieldCondition): string {
  return condition.equals ?? `not-${condition.notEquals ?? ''}`;
}

afterEach(() => cleanup());

describe('specSchema — every kind renders every field of its spec', () => {
  it.each(KIND_SCHEMAS.map((schema) => [schema.kind, schema] as const))('%s', (_kind, schema) => {
    const expected = new Set<string>();
    for (const field of groupFields(schema)) expected.add(field.path);
    for (const section of repeatSections(schema)) {
      for (const field of section.fields) expected.add(`${section.path}[0].${field.path}`);
    }

    // One render per distinct condition (and one with none), each with an item
    // in every repeated group, so conditional fields show in some render.
    const conditions = new Map<string, FieldCondition | null>([['', null]]);
    for (const field of groupFields(schema)) {
      if (field.when) conditions.set(JSON.stringify(field.when), field.when);
    }
    const rendered = new Set<string>();
    for (const condition of conditions.values()) {
      const { container } = renderForm(schema, (buffer) => {
        if (condition) buffer.values[condition.path] = satisfying(condition);
        for (const section of repeatSections(schema)) buffer.repeats[section.path] = [emptyRepeatItem(section)];
      });
      for (const node of container.querySelectorAll('[data-path]')) {
        rendered.add(node.getAttribute('data-path') ?? '');
      }
      // Each field is labelled and holds exactly one control of its kind.
      for (const node of container.querySelectorAll<HTMLElement>('.spec-field')) {
        expect(node.querySelector('label')?.textContent?.trim()).not.toBe('');
        expect(node.querySelectorAll('input, select, textarea').length).toBeGreaterThanOrEqual(1);
      }
      cleanup();
    }

    for (const path of expected) expect(rendered, `field ${path} of ${schema.kind}`).toContain(path);
    expect(rendered).toContain('id');
    expect(rendered).toContain('name');
    expect(rendered).toContain('description');
  });

  it('bounds numbers with the checker limits and lists the closed sets as options', () => {
    const mllp = KIND_SCHEMAS.find((schema) => schema.kind === 'mllp') as KindSchema;
    const { container } = renderForm(mllp);
    const read = container.querySelector<HTMLInputElement>('[data-path="timeouts.read_seconds"] input');
    expect(read?.type).toBe('number');
    expect(read?.min).toBe('1');
    expect(read?.max).toBe('300');
    const bytes = container.querySelector<HTMLInputElement>('[data-path="max_message_bytes"] input');
    expect(bytes?.max).toBe('1048576');
    const ack = container.querySelector<HTMLSelectElement>('[data-path="acknowledgements.mode"] select');
    expect(Array.from(ack?.options ?? []).map((option) => option.value)).toEqual(['', 'application', 'commit']);
    const cidrs = container.querySelector('[data-path="clients.allowed_cidrs"] textarea');
    expect(cidrs).not.toBeNull();
    // The documented defaults, and nothing invented: framing 11/28/13, no timeouts.
    expect(container.querySelector<HTMLInputElement>('[data-path="framing.start_byte"] input')?.value).toBe('11');
    expect(read?.value).toBe('');
  });

  it('hides a field its condition rules out (TLS bindings only for mutual TLS)', () => {
    const mllp = KIND_SCHEMAS.find((schema) => schema.kind === 'mllp') as KindSchema;
    const { container } = renderForm(mllp, (buffer) => (buffer.values['tls.mode'] = 'disabled'));
    expect(container.querySelector('[data-path="tls.server_certificate_binding"]')).toBeNull();
    cleanup();
    const mutual = renderForm(mllp, (buffer) => (buffer.values['tls.mode'] = 'mutual'));
    expect(mutual.container.querySelector('[data-path="tls.server_certificate_binding"]')).not.toBeNull();
  });
});

describe('specSchema — problems land on their fields', () => {
  const mllp = KIND_SCHEMAS.find((schema) => schema.kind === 'mllp') as KindSchema;

  it('puts a problem with a path on that field, marked invalid', () => {
    const { container } = renderForm(mllp, undefined, [
      { code: 'OUT_OF_RANGE', path: 'timeouts.read_seconds', message: 'must be between 1 and 300' }
    ]);
    const field = container.querySelector<HTMLElement>('[data-path="timeouts.read_seconds"]') as HTMLElement;
    expect(within(field).getByText('must be between 1 and 300')).toBeInTheDocument();
    expect(field.querySelector('input')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.queryByTestId('connection-problems')).toBeNull();
  });

  it('puts a list element problem on its list as a numbered entry', () => {
    const { container } = renderForm(mllp, undefined, [
      { code: 'INVALID_CIDR', path: 'clients.allowed_cidrs[1]', message: 'must be a canonical network prefix' }
    ]);
    const field = container.querySelector<HTMLElement>('[data-path="clients.allowed_cidrs"]') as HTMLElement;
    expect(within(field).getByText('Entry 2: must be a canonical network prefix')).toBeInTheDocument();
  });

  it('puts a repeated item field problem on that item, and the item itself in the list', () => {
    const { container } = renderForm(
      mllp,
      (buffer) => {
        buffer.values['tls.mode'] = 'mutual';
        buffer.repeats['clients.identities'] = [emptyRepeatItem(repeatSections(mllp)[0]!)];
      },
      [
        { code: 'REQUIRED', path: 'clients.identities[0].subject', message: 'is required' },
        { code: 'REQUIRED', path: 'clients.identities[0]', message: 'needs a uri_san, an spki_sha256, or both' }
      ]
    );
    const field = container.querySelector<HTMLElement>('[data-path="clients.identities[0].subject"]') as HTMLElement;
    expect(within(field).getByText('is required')).toBeInTheDocument();
    const list = screen.getByTestId('connection-problems');
    expect(list).toHaveTextContent('Client identity 1');
    expect(list).toHaveTextContent('needs a uri_san, an spki_sha256, or both');
  });

  it('lists problems no field shows: the document, a group, an unknown key, a binding', () => {
    renderForm(
      mllp,
      (buffer) => buffer.bindings.push({ name: 'unused', provider: 'env', key: 'X', version: '' }),
      [
        { code: 'INVALID_JSON', path: '', message: 'spec must be a JSON object' },
        { code: 'REQUIRED', path: 'timeouts', message: 'is required' },
        { code: 'UNKNOWN_FIELD', path: 'timeouts.linger_seconds', message: 'is not a field of this connection kind' },
        { code: 'UNUSED_BINDING', path: 'secret_bindings[0].name', message: 'binding "unused" is not named' }
      ]
    );
    const list = screen.getByTestId('connection-problems');
    const items = within(list).getAllByRole('listitem');
    expect(items.map((item) => item.querySelector('.problem-label')?.textContent)).toEqual([
      'Spec',
      'Timeouts',
      'timeouts.linger_seconds',
      'Secret binding unused · name'
    ]);
    // Only an unused binding is a warning; everything else blocks compile.
    expect(items.map((item) => item.getAttribute('data-tone'))).toEqual(['danger', 'danger', 'danger', 'warning']);
  });
});

describe('specSchema — binding fields name declared bindings, and nothing can be typed there', () => {
  const s3 = KIND_SCHEMAS.find((schema) => schema.kind === 'batch_s3') as KindSchema;

  it('offers exactly the declared binding names in every *_binding field', () => {
    const { container } = renderForm(s3, (buffer) => {
      buffer.bindings.push(
        { name: 'batch-s3-access-key', provider: 'env', key: 'FI_FHIR_BATCH_S3_ACCESS_KEY', version: '' },
        { name: 'batch-s3-secret-key', provider: 'env', key: 'FI_FHIR_BATCH_S3_SECRET_KEY', version: '' }
      );
    });
    for (const path of ['s3.access_key_binding', 's3.secret_access_key_binding']) {
      const field = container.querySelector<HTMLElement>(`[data-path="${path}"]`) as HTMLElement;
      // A Select and nothing else: there is no text box to paste a secret into.
      expect(field.querySelector('input, textarea')).toBeNull();
      const select = field.querySelector<HTMLSelectElement>('select');
      const labels = Array.from(select?.options ?? []).map((option) => option.textContent?.trim());
      expect(labels).toEqual(['Choose a binding', 'batch-s3-access-key', 'batch-s3-secret-key']);
    }
  });

  it('lets an optional binding go back to none', () => {
    const https = KIND_SCHEMAS.find((schema) => schema.kind === 'https') as KindSchema;
    const { container } = renderForm(https, (buffer) => {
      buffer.bindings.push({ name: 'sandbox-token', provider: 'env', key: 'FHIR_SANDBOX_TOKEN', version: '' });
    });
    const optional = container.querySelector<HTMLSelectElement>('[data-path="https.ca_bundle_binding"] select');
    expect(Array.from(optional?.options ?? []).map((option) => option.textContent?.trim())).toEqual([
      'None',
      'sandbox-token'
    ]);
    const required = container.querySelector<HTMLSelectElement>('[data-path="https.token_binding"] select');
    expect(required?.options[0]?.textContent?.trim()).toBe('Choose a binding');
    expect(required?.options[0]?.disabled).toBe(true);
  });

  it('shows a stored name no binding declares as a read-only option, so UNBOUND_SECRET lands on it', () => {
    const { container } = renderForm(
      s3,
      (buffer) => {
        buffer.bindings.push({ name: 'declared', provider: 'env', key: 'K', version: '' });
        buffer.values['s3.access_key_binding'] = 'not-declared';
      },
      [{ code: 'UNBOUND_SECRET', path: 's3.access_key_binding', message: 'names secret binding "not-declared", which is not declared' }]
    );
    const field = container.querySelector<HTMLElement>('[data-path="s3.access_key_binding"]') as HTMLElement;
    expect(field.querySelector('input')).toBeNull();
    const select = field.querySelector<HTMLSelectElement>('select') as HTMLSelectElement;
    expect(select.value).toBe('not-declared');
    const shown = Array.from(select.options).find((option) => option.value === 'not-declared');
    expect(shown?.textContent?.trim()).toBe('not-declared (not declared)');
    expect(shown?.disabled).toBe(true);
    expect(within(field).getByText(/which is not declared/)).toBeInTheDocument();
    expect(select).toHaveAttribute('aria-invalid', 'true');
  });

  it('is a disabled Select saying to declare a binding first when none is declared', () => {
    const { container } = renderForm(s3);
    const field = container.querySelector<HTMLElement>('[data-path="s3.access_key_binding"]') as HTMLElement;
    expect(field.querySelector('input')).toBeNull();
    const select = field.querySelector<HTMLSelectElement>('select') as HTMLSelectElement;
    expect(select.disabled).toBe(true);
    expect(select).toHaveAttribute('title', BINDING_DECLARE_FIRST);
    expect(within(field).getByText(BINDING_DECLARE_FIRST)).toBeInTheDocument();
  });
});

describe('specSchema — form-only choices and fixed values', () => {
  const sftp = KIND_SCHEMAS.find((schema) => schema.kind === 'batch_sftp') as KindSchema;
  const fhir = KIND_SCHEMAS.find((schema) => schema.kind === 'fhir') as KindSchema;

  it('makes SFTP "exactly one of password and private key" the shape of the form', () => {
    const paths = (credential: string) => {
      const { container } = renderForm(sftp, (buffer) => (buffer.values[SFTP_CREDENTIAL] = credential));
      const shown = ['sftp.password_binding', 'sftp.private_key_binding', 'sftp.private_key_passphrase_binding'].filter(
        (path) => container.querySelector(`[data-path="${path}"]`) !== null
      );
      cleanup();
      return shown;
    };
    expect(paths('')).toEqual([]);
    expect(paths('password')).toEqual(['sftp.password_binding']);
    expect(paths('private_key')).toEqual(['sftp.private_key_binding', 'sftp.private_key_passphrase_binding']);
  });

  it('reads the SFTP credential choice back from the stored spec and never writes it', () => {
    const stored = { sftp: { host: 'h', password_binding: 'sftp-password' } };
    const { values, repeats } = valuesFromSpec(sftp, stored);
    expect(values[SFTP_CREDENTIAL]).toBe('password');
    const spec = buildSpec(sftp, values, repeats, stored);
    expect(spec).toEqual(stored);
    values[SFTP_CREDENTIAL] = 'private_key';
    values['sftp.private_key_binding'] = 'sftp-key';
    expect(buildSpec(sftp, values, repeats, stored)).toEqual({ sftp: { host: 'h', private_key_binding: 'sftp-key' } });
  });

  it('shows the FHIR interaction as its one allowed value, read only, and always writes it', () => {
    const { container } = renderForm(fhir);
    const input = container.querySelector<HTMLInputElement>('[data-path="fhir.interaction"] input');
    expect(input?.value).toBe('transaction');
    expect(input?.readOnly).toBe(true);
    expect(container.querySelector('[data-path="fhir.interaction"] select')).toBeNull();
    const { values, repeats } = valuesFromSpec(fhir, { fhir: { base_url: 'https://fhir.example/r4' } });
    values['fhir.interaction'] = 'batch';
    expect(buildSpec(fhir, values, repeats, {})).toEqual({
      fhir: { base_url: 'https://fhir.example/r4', interaction: 'transaction' }
    });
  });
});

describe('specSchema — the form model carries C-0 documents unchanged', () => {
  const entries = Object.entries(FIXTURES);

  it('finds the seven C-0 fixtures', () => {
    expect(entries.map(([, fixture]) => fixture.kind).sort()).toEqual(
      ['batch_s3', 'batch_sftp', 'fhir', 'http', 'https', 'kafka', 'mllp']
    );
  });

  it.each(entries.map(([file, fixture]) => [file.split('/').pop(), fixture] as const))(
    '%s round-trips through the form values',
    (_file, fixture) => {
      const schema = KIND_SCHEMAS.find((candidate) => candidate.kind === fixture.kind) as KindSchema;
      const { values, repeats } = valuesFromSpec(schema, fixture.spec);
      // No base: every key must come back from a field the form renders.
      expect(buildSpec(schema, values, repeats, {})).toEqual(fixture.spec);
    }
  );

  it('keeps keys the form does not know, so the checker can name them', () => {
    const kafka = KIND_SCHEMAS.find((schema) => schema.kind === 'kafka') as KindSchema;
    const base = { destination_id: 'd', class: 'sandbox', kafka: { topic: 't', partitions: 3 }, extra: true };
    const { values, repeats } = valuesFromSpec(kafka, base);
    expect(buildSpec(kafka, values, repeats, base)).toEqual(base);
  });

  it('drops a hidden field and an emptied group', () => {
    const http = KIND_SCHEMAS.find((schema) => schema.kind === 'http') as KindSchema;
    const stored = {
      source_id: 'adt',
      auth_mode: 'oauth2',
      oauth: { issuer_url: 'https://issuer.example', audience: 'a', allowed_client_ids: ['c'] },
      max_body_bytes: 10
    };
    const { values, repeats } = valuesFromSpec(http, stored);
    values['auth_mode'] = 'bearer';
    const spec = buildSpec(http, values, repeats, stored);
    expect(spec).not.toHaveProperty('oauth');
    expect(spec['auth_mode']).toBe('bearer');
  });
});

describe('specSchema — controls to JSON', () => {
  const field = (control: SpecField['control']): SpecField => ({ path: 'x', label: 'X', control });

  it('converts each control and leaves an empty one out', () => {
    expect(controlValue(field('number'), '42')).toBe(42);
    expect(controlValue(field('number'), 42)).toBe(42);
    expect(controlValue(field('number'), null)).toBeUndefined();
    expect(controlValue(field('number'), '4.5x')).toBe('4.5x');
    expect(controlValue(field('boolean'), 'false')).toBe(false);
    expect(controlValue(field('boolean'), '')).toBeUndefined();
    expect(controlValue(field('list'), '10.0.0.0/8\n\n 192.168.0.0/16 \n')).toEqual(['10.0.0.0/8', '192.168.0.0/16']);
    expect(controlValue(field('list'), '  \n')).toBeUndefined();
    expect(controlValue(field('text'), '  adt-east  ')).toBe('adt-east');
    expect(controlValue(field('binding'), '')).toBeUndefined();
  });

  it('names each kind\'s endpoint from its spec, and nothing when it has none', () => {
    expect(endpointOf('MLLP', { listen_address: '0.0.0.0:2575' })).toBe('0.0.0.0:2575');
    expect(endpointOf('BATCH_S3', { s3: { endpoint: 'minio:9000', bucket: 'b', input_prefix: 'in' } })).toBe('minio:9000/b/in');
    expect(endpointOf('BATCH_SFTP', { sftp: { host: 'h', port: 22, input_directory: '/in' } })).toBe('h:22/in');
    expect(endpointOf('KAFKA', { kafka: { topic: 'integration.delivery.v1' } })).toBe('integration.delivery.v1');
    expect(endpointOf('HTTP', {})).toBe('');
  });
});
