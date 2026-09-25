import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import DestinationDeliveries from './DestinationDeliveries.svelte';

const fhirDelivered = {
  transport: 'fhir',
  outcome: 'delivered',
  endpointAdvisory: 'https://fhir.example.test/r4',
  fhirResourceTypes: ['Patient', 'Encounter'],
  fhirEntryCount: 2,
  fhirOutcomeCodesAdvisory: []
};

const fhirRefused = {
  ...fhirDelivered,
  outcome: 'refused',
  fhirOutcomeCodesAdvisory: ['invalid', 'not-found']
};

const httpsRetryable = {
  transport: 'https',
  outcome: 'retryable',
  endpointAdvisory: 'https://destination.example.test/ingest',
  fhirResourceTypes: [],
  fhirEntryCount: 0,
  fhirOutcomeCodesAdvisory: []
};

describe('DestinationDeliveries', () => {
  it('renders each ledger row newest first with transport, outcome, types, count, codes, and endpoint', () => {
    render(DestinationDeliveries, {
      props: { deliveries: [fhirDelivered, fhirRefused, httpsRetryable], label: 'Deliveries for attempt-a' }
    });

    const list = screen.getByRole('list', { name: 'Deliveries for attempt-a' });
    const rows = within(list).getAllByRole('listitem').filter((item) => item.parentElement === list);
    expect(rows).toHaveLength(3);
    const row = (index: number) => {
      const element = rows[index];
      if (!element) throw new Error(`no delivery row ${index}`);
      return within(element);
    };

    const delivered = row(0);
    expect(delivered.getByText('FHIR')).toBeInTheDocument();
    expect(delivered.getByText('Delivered')).toBeInTheDocument();
    expect(delivered.getByText('2 bundle entries')).toBeInTheDocument();
    const types = delivered.getByRole('list', { name: 'FHIR resource types' });
    expect(within(types).getAllByRole('listitem').map((item) => item.textContent)).toEqual([
      'Patient',
      'Encounter'
    ]);
    expect(delivered.queryByRole('list', { name: 'OperationOutcome issue codes' })).toBeNull();
    expect(delivered.getByText('https://fhir.example.test/r4')).toBeInTheDocument();

    const refused = row(1);
    expect(refused.getByText('Refused')).toBeInTheDocument();
    const codes = refused.getByRole('list', { name: 'OperationOutcome issue codes' });
    expect(within(codes).getAllByRole('listitem').map((item) => item.textContent)).toEqual([
      'invalid',
      'not-found'
    ]);

    const https = row(2);
    expect(https.getByText('HTTPS')).toBeInTheDocument();
    expect(https.getByText('Retryable failure')).toBeInTheDocument();
    expect(https.queryByRole('list', { name: 'FHIR resource types' })).toBeNull();
    expect(https.queryByText(/bundle entr/)).toBeNull();
    expect(https.getByText('https://destination.example.test/ingest')).toBeInTheDocument();
  });

  it('states honestly that no destination was contacted when the ledger has no row', () => {
    render(DestinationDeliveries, { props: { deliveries: [] } });
    expect(screen.getByText(/contacted no destination for this attempt/i)).toBeInTheDocument();
    expect(screen.queryByRole('list')).toBeNull();
  });

  it('renders the six ledger facts and nothing else, even if a row carried more', () => {
    // A defensive check on the PHI posture: the component must not become a
    // generic object renderer that would surface a field someone later adds.
    const widened = {
      ...fhirRefused,
      diagnostics: 'Patient DOE^JOHN MRN 12345 failed validation',
      failureCode: 'DELIVERY_DESTINATION_REJECTED',
      servedCertificateSubjectAdvisory: 'CN=fhir.example.test'
    };
    const { container } = render(DestinationDeliveries, { props: { deliveries: [widened] } });
    expect(container.textContent).not.toMatch(/DOE\^JOHN|MRN 12345/);
    expect(container.textContent).not.toMatch(/DELIVERY_DESTINATION_REJECTED/);
    expect(container.textContent).not.toMatch(/CN=fhir\.example\.test/);
  });
});
