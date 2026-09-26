/**
 * Legacy-syntax parents (most of the app) must be able to consume the runes
 * primitives: onclick props, default content as children, {#snippet} regions.
 */
import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import LegacyConsumer from './__fixtures__/LegacyConsumer.svelte';
import * as primitives from './index';

describe('primitives from a legacy-syntax parent', () => {
  it('renders snippets and default content, and fires onclick/onchange props', async () => {
    const onRun = vi.fn();
    const onTab = vi.fn();
    render(LegacyConsumer, { props: { onRun, onTab } });

    expect(screen.getByRole('heading', { level: 1, name: 'Legacy page' })).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Summary' })).toHaveTextContent('View: browse');

    await fireEvent.click(screen.getByRole('button', { name: 'Run' }));
    expect(onRun).toHaveBeenCalledOnce();

    await fireEvent.click(screen.getByRole('tab', { name: 'Statistics' }));
    expect(onTab).toHaveBeenCalledWith('stats');
    expect(screen.getByTestId('legacy-body')).toHaveTextContent('View: stats');
  });

  it('exports every primitive from the package index', () => {
    expect(Object.keys(primitives).sort()).toEqual([
      'Badge',
      'Button',
      'EmptyState',
      'Field',
      'Icon',
      'IconButton',
      'Input',
      'KeyValue',
      'Panel',
      'Popover',
      'Select',
      'Table',
      'Tabs',
      'Td',
      'Textarea',
      'Th',
      'Toolbar',
      'Tr'
    ]);
  });
});
