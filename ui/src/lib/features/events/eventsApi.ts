/**
 * Events API — GraphQL client functions for event browsing, statistics, and patient timelines.
 */

import {
  EventsDocument,
  EventByIdDocument,
  EventStatisticsDocument,
  PatientTimelineDocument,
  PatientsDocument,
  type EventsQuery,
  type EventByIdQuery,
  type EventStatisticsQuery,
  type PatientTimelineQuery,
  type PatientsQuery,
  type EventFilter,
  type EventOrderBy,
  type PatientFilter
} from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

/**
 * Query events with filters, pagination, and ordering.
 */
export async function queryEvents(
  filter?: EventFilter | null,
  first?: number,
  after?: string | null,
  orderBy?: EventOrderBy | null
): Promise<EventsQuery['events']> {
  const result = await graphqlFetch(EventsDocument, {
    filter: filter ?? null,
    first: first ?? 50,
    after: after ?? null,
    orderBy: orderBy ?? null
  });
  return result.events;
}

/**
 * Get a single event by ID.
 */
export async function getEvent(
  id: string
): Promise<EventByIdQuery['event']> {
  const result = await graphqlFetch(EventByIdDocument, { id });
  return result.event;
}

/**
 * Get aggregate event statistics (total, by type, by source).
 */
export async function getEventStatistics(): Promise<EventStatisticsQuery['eventStatistics']> {
  const result = await graphqlFetch(EventStatisticsDocument, {});
  return result.eventStatistics;
}

/**
 * Get a patient's event timeline (chronological history).
 */
export async function getPatientTimeline(
  mrn: string,
  fromTimestamp?: string | null,
  toTimestamp?: string | null,
  limit?: number
): Promise<PatientTimelineQuery['patientTimeline']> {
  const result = await graphqlFetch(PatientTimelineDocument, {
    mrn,
    fromTimestamp: fromTimestamp ?? null,
    toTimestamp: toTimestamp ?? null,
    limit: limit ?? 100
  });
  return result.patientTimeline;
}

/**
 * Query patients with filter and pagination.
 */
export async function queryPatients(
  filter?: PatientFilter | null,
  first?: number,
  after?: string | null
): Promise<PatientsQuery['patients']> {
  const result = await graphqlFetch(PatientsDocument, {
    filter: filter ?? null,
    first: first ?? 50,
    after: after ?? null
  });
  return result.patients;
}
