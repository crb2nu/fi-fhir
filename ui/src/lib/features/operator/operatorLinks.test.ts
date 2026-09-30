import { describe, expect, it } from 'vitest';
import {
  connectionHref,
  eventsReceiptHref,
  operatorAttemptHref,
  operatorDeploymentHref,
  operatorReceiptHref,
  parseOperatorDeepLink
} from './operatorLinks';

describe('parseOperatorDeepLink', () => {
  it('reads each documented form', () => {
    expect(parseOperatorDeepLink('?receipt=rcpt-1')).toEqual({ tab: 'messages', receiptId: 'rcpt-1' });
    expect(parseOperatorDeepLink('?attempt=att-1')).toEqual({ tab: 'delivery', attemptId: 'att-1' });
    expect(parseOperatorDeepLink('?definition=def-a&revision=v1')).toEqual({
      tab: 'deployments',
      definitionId: 'def-a',
      revisionId: 'v1'
    });
  });

  it('prefers a receipt, then an attempt, when several are named', () => {
    expect(parseOperatorDeepLink('?attempt=a&receipt=r')).toEqual({ tab: 'messages', receiptId: 'r' });
    expect(parseOperatorDeepLink('?definition=d&revision=v&attempt=a')).toEqual({
      tab: 'delivery',
      attemptId: 'a'
    });
  });

  it('ignores half a deployment address, empty values, and oversized or control-character ids', () => {
    expect(parseOperatorDeepLink('?definition=def-a')).toBeNull();
    expect(parseOperatorDeepLink('?revision=v1')).toBeNull();
    expect(parseOperatorDeepLink('?receipt=')).toBeNull();
    expect(parseOperatorDeepLink(`?receipt=${'x'.repeat(257)}`)).toBeNull();
    expect(parseOperatorDeepLink('?receipt=a%0Ab')).toBeNull();
    expect(parseOperatorDeepLink('')).toBeNull();
  });
});

describe('outbound links', () => {
  it('encodes ids into the documented query forms', () => {
    expect(connectionHref('fhir primary')).toBe('/connections?connection=fhir+primary');
    expect(eventsReceiptHref('rcpt/1')).toBe('/events?receipt=rcpt%2F1');
    expect(operatorReceiptHref('r')).toBe('/operator?receipt=r');
    expect(operatorAttemptHref('a')).toBe('/operator?attempt=a');
    expect(operatorDeploymentHref('d', 'v1')).toBe('/operator?definition=d&revision=v1');
  });

  it('round-trips through the parser', () => {
    const href = operatorDeploymentHref('def a', 'rev&1');
    expect(parseOperatorDeepLink(href.slice(href.indexOf('?')))).toEqual({
      tab: 'deployments',
      definitionId: 'def a',
      revisionId: 'rev&1'
    });
  });
});
