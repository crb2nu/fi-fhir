export type HL7Sample = {
  id: string;
  name: string;
  source: string;
  raw: string;
  createdAt: string; // ISO
  messageType?: string;
  controlId?: string;
  version?: string;
};

export type NewHL7Sample = {
  name?: string;
  source: string;
  raw: string;
};

