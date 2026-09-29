import { describe, expect, it } from 'vitest';
import { connectionHref, definitionHref, operatorReceiptHref, parseVerificationDeepLink } from './verificationLinks';

describe('verification deep links', () => {
  it('reads ?receipt= and refuses what the server would refuse', () => {
    expect(parseVerificationDeepLink('?receipt=receipt-1')).toEqual({ receiptId: 'receipt-1' });
    expect(parseVerificationDeepLink('?receipt=%20receipt-1%20')).toEqual({ receiptId: 'receipt-1' });
    expect(parseVerificationDeepLink('')).toBeNull();
    expect(parseVerificationDeepLink('?receipt=')).toBeNull();
    expect(parseVerificationDeepLink(`?receipt=${'r'.repeat(257)}`)).toBeNull();
    expect(parseVerificationDeepLink('?receipt=a%00b')).toBeNull();
  });

  it('links out to the trace, the connection and the definition', () => {
    expect(operatorReceiptHref('receipt 1')).toBe('/operator?receipt=receipt+1');
    expect(connectionHref('source-adt')).toBe('/connections?connection=source-adt');
    expect(definitionHref('adt-east', '1')).toBe('/connections?definition=adt-east&revision=1');
  });
});
