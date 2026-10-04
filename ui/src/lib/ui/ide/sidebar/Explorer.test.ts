import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { createWorkspaceTab, openTab, resetIDEState } from '../ideStore';
import type { ConnectionRow } from '$lib/features/connections/connectionsApi';

const api = vi.hoisted(() => ({ fetchConnections: vi.fn() }));
vi.mock('$lib/features/connections/connectionsApi', () => api);
vi.mock('$lib/features/integration-session/api', () => ({ isIntegrationSessionBuildEnabled: () => false }));
const { default: Explorer } = await import('./Explorer.svelte');

function status(capabilities: Record<string, boolean> = {}, principal = 'operator') {
  return {
    authenticated: true, authVia: 'network', principal,
    capabilities: { operatorRead: true, connectionsRead: true, controlPlane: true, connectionCatalog: true, ...capabilities },
    missingRoles: { connectionsRead: ['integration.operator'] },
  };
}

function connection(id: string, direction: ConnectionRow['direction'] = 'SOURCE', name = id): ConnectionRow {
  return {
    id, name, direction, kind: direction === 'SOURCE' ? 'MLLP' : 'HTTPS', description: '',
    spec: {}, secretBindings: [], version: 1, archived: false, latestRevision: null, references: [],
    runtime: { mounted: false, role: null, detail: null, revisionId: null, digest: null },
    createdBy: { id: 'operator', kind: 'service' }, createdAt: '2026-10-03T10:00:00Z',
    updatedBy: { id: 'operator', kind: 'service' }, updatedAt: '2026-10-03T10:00:00Z', updatedReason: 'Test connection',
  };
}

const onnavigate = vi.fn();
const props = { onclose: vi.fn(), onnavigate };
const catalog = () => screen.getByRole('region', { name: 'Connection catalog' });

beforeEach(() => {
  localStorage.clear();
  resetIDEState();
  setAccessStatus(status());
  vi.clearAllMocks();
  api.fetchConnections.mockReset().mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  resetAccessCapabilities();
});

describe('Explorer connections', () => {
  it.each([
    [{ controlPlane: false }, 'not configured'],
    [{ connectionCatalog: false }, 'not configured'],
    [{ connectionsRead: false }, 'integration.operator'],
  ])('does not query a blocked catalog: %j', async (capabilities, message) => {
    setAccessStatus(status(capabilities));
    render(Explorer, { props });
    expect(catalog()).toHaveTextContent(message);
    expect(api.fetchConnections).not.toHaveBeenCalled();
    expect(within(catalog()).queryByRole('button', { name: 'Refresh connections' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Connections' })).toBeInTheDocument();
  });

  it('tries unknown capability, reports failure, and retries to honest empty groups', async () => {
    resetAccessCapabilities();
    api.fetchConnections.mockRejectedValueOnce(new Error('connection catalog unavailable'));
    render(Explorer, { props });
    const retry = await within(catalog()).findByRole('button', { name: 'Retry connections' });
    expect(catalog()).toHaveTextContent('The connection catalog is unavailable');
    expect(catalog()).not.toHaveTextContent('No source connections');
    expect(api.fetchConnections).toHaveBeenCalledWith(null, false);
    await fireEvent.click(retry);
    expect(await within(catalog()).findByText('No source connections yet.')).toBeInTheDocument();
    expect(within(catalog()).getByText('No destination connections yet.')).toBeInTheDocument();
    expect(api.fetchConnections).toHaveBeenCalledTimes(2);
  });

  it('partitions actual catalog rows, encodes links, and marks only the active connection', async () => {
    const id = 'east/source & one';
    api.fetchConnections.mockResolvedValue([
      connection(id, 'SOURCE', 'ADT east'), connection('fhir-west', 'DESTINATION', 'FHIR west'),
      { ...connection('retired'), archived: true },
    ]);
    openTab(createWorkspaceTab('/connections', 'connections', `?connection=${encodeURIComponent(id)}`));
    render(Explorer, { props });
    const source = await screen.findByRole('link', { name: 'ADT east' });
    const destination = screen.getByRole('link', { name: 'FHIR west' });
    expect(within(screen.getByRole('navigation', { name: 'Source connections' })).getByRole('link', { name: 'ADT east' })).toBe(source);
    expect(within(screen.getByRole('navigation', { name: 'Destination connections' })).getByRole('link', { name: 'FHIR west' })).toBe(destination);
    expect(source).toHaveAttribute('href', '/connections?connection=east%2Fsource+%26+one');
    expect(source).toHaveAttribute('title', `ADT east · ${id}`);
    expect(source).toHaveAttribute('aria-current', 'page');
    expect(destination).not.toHaveAttribute('aria-current');
    expect(screen.queryByRole('link', { name: 'retired' })).not.toBeInTheDocument();
    await fireEvent.click(source, { ctrlKey: true });
    expect(onnavigate).not.toHaveBeenCalled();
    await fireEvent.click(source);
    expect(onnavigate).toHaveBeenCalledWith('/connections?connection=east%2Fsource+%26+one');

    openTab(createWorkspaceTab('/connections', 'connections', '?connection=fhir-west'));
    await waitFor(() => expect(destination).toHaveAttribute('aria-current', 'page'));
    expect(source).not.toHaveAttribute('aria-current');
    openTab(createWorkspaceTab('/connections', 'connections', '?connection=fhir-west&definition=other&revision=1'));
    await waitFor(() => expect(destination).not.toHaveAttribute('aria-current'));
    openTab(createWorkspaceTab('/operator'));
    await waitFor(() => expect(destination).not.toHaveAttribute('aria-current'));
    expect(localStorage.getItem('fi-fhir-ide-layout')).not.toContain('ADT east');
  });

  it('bounds visible rows and searches the full returned inventory through collapsed groups', async () => {
    api.fetchConnections.mockResolvedValue(Array.from({ length: 12 }, (_, i) =>
      connection(`source-${i}`, 'SOURCE', `Source ${String(i).padStart(2, '0')}`)));
    const view = render(Explorer, { props });
    await screen.findByRole('link', { name: 'Source 00' });
    expect(within(screen.getByRole('navigation', { name: 'Source connections' })).getAllByRole('link')).toHaveLength(9);
    expect(catalog()).toHaveTextContent('Showing 8 of 12. Filter to find a connection or open Connections.');
    expect(screen.queryByRole('link', { name: 'Source 11' })).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Sources 12' }));
    expect(screen.queryByRole('link', { name: 'Source 00' })).not.toBeInTheDocument();
    const filter = screen.getByRole('textbox', { name: 'Filter explorer' });
    await fireEvent.input(filter, { target: { value: 'source-11' } });
    expect(screen.getByRole('link', { name: 'Source 11' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sources 1' })).toBeDisabled();
    await fireEvent.input(filter, { target: { value: 'Source 10' } });
    expect(screen.getByRole('link', { name: 'Source 10' })).toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Clear explorer filter' }));
    expect(filter).toHaveFocus();
    expect(screen.queryByRole('link', { name: 'Source 00' })).not.toBeInTheDocument();
    view.unmount();
    render(Explorer, { props });
    expect(await screen.findByRole('button', { name: 'Sources 12' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('explains the catalog limit without rendering hundreds of rows', async () => {
    api.fetchConnections.mockResolvedValue(Array.from({ length: 500 }, (_, i) => connection(`source-${i}`)));
    render(Explorer, { props });
    expect(await screen.findByText(/Only the first 500 connections/)).toBeInTheDocument();
    expect(within(catalog()).getAllByRole('link')).toHaveLength(10);
    await fireEvent.input(screen.getByRole('textbox', { name: 'Filter explorer' }), { target: { value: 'source-499' } });
    expect(screen.getByRole('link', { name: 'source-499' })).toBeInTheDocument();
  });

  it('keeps the active connection visible within the compact row limit', async () => {
    api.fetchConnections.mockResolvedValue(Array.from({ length: 12 }, (_, i) =>
      connection(`source-${i}`, 'SOURCE', `Source ${String(i).padStart(2, '0')}`)));
    openTab(createWorkspaceTab('/connections', 'connections', '?connection=source-11'));
    render(Explorer, { props });
    expect(await screen.findByRole('link', { name: 'Source 11' })).toHaveAttribute('aria-current', 'page');
    expect(within(screen.getByRole('navigation', { name: 'Source connections' })).getAllByRole('link')).toHaveLength(9);
    expect(screen.queryByRole('link', { name: 'Source 07' })).not.toBeInTheDocument();
  });

  it('shows loading and removes old rows while refreshing, then exposes retry on failure', async () => {
    api.fetchConnections.mockResolvedValueOnce([connection('east', 'SOURCE', 'ADT east')]);
    render(Explorer, { props });
    await screen.findByRole('link', { name: 'ADT east' });
    let reject!: (reason: Error) => void;
    api.fetchConnections.mockReturnValueOnce(new Promise((_, no) => { reject = no; }));
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh connections' }));
    expect(within(catalog()).getByRole('status')).toHaveTextContent('Loading connections');
    expect(screen.getByRole('button', { name: 'Refresh connections' })).toBeDisabled();
    expect(screen.queryByRole('link', { name: 'ADT east' })).not.toBeInTheDocument();
    reject(new Error('offline'));
    const retry = await screen.findByRole('button', { name: 'Retry connections' });
    expect(catalog()).not.toHaveTextContent('No source connections');
    api.fetchConnections.mockResolvedValueOnce([connection('west', 'DESTINATION', 'FHIR west')]);
    await fireEvent.click(retry);
    expect(await screen.findByRole('link', { name: 'FHIR west' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'ADT east' })).not.toBeInTheDocument();
  });

  it('ignores an in-flight result after access is revoked and reloads for a new identity', async () => {
    let finish!: (rows: ConnectionRow[]) => void;
    api.fetchConnections.mockReturnValueOnce(new Promise((yes) => { finish = yes; }));
    render(Explorer, { props });
    setAccessStatus(status({ connectionsRead: false }));
    await waitFor(() => expect(catalog()).toHaveTextContent('integration.operator'));
    finish([connection('east', 'SOURCE', 'ADT east')]);
    await waitFor(() => expect(screen.queryByRole('link', { name: 'ADT east' })).not.toBeInTheDocument());
    api.fetchConnections.mockResolvedValueOnce([connection('west', 'DESTINATION', 'FHIR west')]);
    setAccessStatus(status({}, 'second-operator'));
    expect(await screen.findByRole('link', { name: 'FHIR west' })).toBeInTheDocument();
    expect(api.fetchConnections).toHaveBeenCalledTimes(2);
  });
});
