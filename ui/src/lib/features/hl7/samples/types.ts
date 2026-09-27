import type { HL7RedactionMode } from '$lib/domain/hl7Redact';

/**
 * A sample that lives in the page's Integration Session because it was
 * captured or peeked from a source connection (.loom/38 C-3). Preview runs it
 * by `sampleId` instead of adding the editor text again.
 */
export type HL7SessionSampleRef = {
  sessionId: string;
  sampleId: string;
  /** The session sample's `source`: `capture:<captureId>` or `peek:<captureId>`. */
  provenance: string;
  /** The server withheld the text (`redactedPayload` null): the caller lacks integration.operator. */
  payloadWithheld: boolean;
};

export type HL7Sample = {
  id: string;
  name: string;
  source: string;
  feed?: string;
  tags?: string[];
  redactionMode?: HL7RedactionMode;
  raw: string;
  createdAt: string; // ISO
  messageType?: string;
  controlId?: string;
  version?: string;
  session?: HL7SessionSampleRef | undefined;
};

export type NewHL7Sample = {
  name?: string;
  source: string;
  feed?: string;
  tags?: string[];
  redactionMode?: HL7RedactionMode;
  raw: string;
};
