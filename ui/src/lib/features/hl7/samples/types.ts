import type { HL7RedactionMode } from '$lib/domain/hl7Redact';

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
};

export type NewHL7Sample = {
  name?: string;
  source: string;
  feed?: string;
  tags?: string[];
  redactionMode?: HL7RedactionMode;
  raw: string;
};
