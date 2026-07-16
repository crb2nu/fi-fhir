/**
 * MermaidDiagram tests — lazy mermaid rendering with graceful fallback.
 * The mermaid module is mocked: what's under test is the component's
 * contract (strict security config, svg insertion, raw-source fallback),
 * not mermaid's own rendering.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';

const mockRender = vi.fn();
const mockInitialize = vi.fn();

vi.mock('mermaid', () => ({
  default: {
    initialize: (...args: unknown[]) => mockInitialize(...args),
    render: (...args: unknown[]) => mockRender(...args)
  }
}));

import MermaidDiagram from './MermaidDiagram.svelte';

beforeEach(() => {
  mockRender.mockReset();
  mockInitialize.mockReset();
});

describe('MermaidDiagram', () => {
  it('renders the mermaid SVG output with strict security config', async () => {
    mockRender.mockResolvedValue({ svg: '<svg data-testid="diagram-svg"></svg>' });

    render(MermaidDiagram, { props: { source: 'graph TD; A-->B;' } });

    await waitFor(() => expect(screen.getByTestId('diagram-svg')).toBeInTheDocument());
    expect(mockInitialize).toHaveBeenCalledWith(
      expect.objectContaining({ securityLevel: 'strict', startOnLoad: false })
    );
    const [, source] = mockRender.mock.calls[0]!;
    expect(source).toBe('graph TD; A-->B;');
  });

  it('falls back to the raw source when mermaid cannot render it', async () => {
    mockRender.mockRejectedValue(new Error('Parse error on line 1'));

    render(MermaidDiagram, { props: { source: 'not a diagram' } });

    await waitFor(() =>
      expect(screen.getByText('Diagram could not be rendered')).toBeInTheDocument()
    );
    expect(screen.getByText('not a diagram')).toBeInTheDocument();
  });

  it('renders nothing for blank source', async () => {
    const { container } = render(MermaidDiagram, { props: { source: '   ' } });

    // Give the (skipped) async path a tick to settle.
    await new Promise((r) => setTimeout(r, 0));
    expect(mockRender).not.toHaveBeenCalled();
    expect(container.querySelector('.mermaid-diagram')).toBeNull();
    expect(container.querySelector('.mermaid-loading')).toBeNull();
  });

  it('shows a loading placeholder until the render resolves', async () => {
    let resolveRender: (v: { svg: string }) => void = () => {};
    mockRender.mockReturnValue(new Promise((r) => (resolveRender = r)));

    render(MermaidDiagram, { props: { source: 'graph TD; A-->B;' } });

    expect(await screen.findByText('Rendering diagram…')).toBeInTheDocument();
    resolveRender({ svg: '<svg data-testid="late-svg"></svg>' });
    await waitFor(() => expect(screen.getByTestId('late-svg')).toBeInTheDocument());
    expect(screen.queryByText('Rendering diagram…')).toBeNull();
  });
});
