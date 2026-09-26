import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import FieldFixture from './__fixtures__/FieldFixture.svelte';
import Input from './Input.svelte';

describe('Field + Input / Select / Textarea', () => {
  it('labels the Input and describes it with the hint', () => {
    render(FieldFixture, { props: { kind: 'input', label: 'Endpoint', hint: 'FHIR R4 base URL.' } });
    const input = screen.getByLabelText('Endpoint');
    expect(input).toHaveAttribute('data-testid', 'control');
    expect(input).toHaveClass('ui-input', 'ui-input--sm', 'is-mono');
    expect(input).toHaveAccessibleDescription('FHIR R4 base URL.');
    expect(input).not.toHaveAttribute('aria-invalid');
  });

  it('replaces the hint with the error and marks the control invalid', () => {
    render(FieldFixture, {
      props: { kind: 'input', hint: 'ignored', error: 'Must be an https URL.', required: true }
    });
    const input = screen.getByLabelText(/Endpoint/);
    expect(input).toHaveAttribute('aria-invalid', 'true');
    expect(input).toBeRequired();
    expect(input).toHaveAccessibleDescription('Must be an https URL.');
    expect(screen.queryByText('ignored')).toBeNull();
    expect(screen.getByTestId('field')).toHaveClass('is-invalid');
  });

  it('binds Input values both ways', async () => {
    const onvalue = vi.fn();
    render(FieldFixture, { props: { kind: 'input', onvalue } });
    const input = screen.getByLabelText('Endpoint') as HTMLInputElement;
    expect(input.value).toBe('initial');
    await fireEvent.input(input, { target: { value: 'https://fhir.example.test' } });
    expect(onvalue).toHaveBeenLastCalledWith('https://fhir.example.test');
  });

  it('renders Select options, honours disabled options and binds the value', async () => {
    const onvalue = vi.fn();
    render(FieldFixture, { props: { kind: 'select', label: 'Retry', onvalue } });
    const select = screen.getByLabelText('Retry') as HTMLSelectElement;
    expect(select.value).toBe('b');
    expect(screen.getByRole('option', { name: 'Charlie' })).toBeDisabled();
    await fireEvent.change(select, { target: { value: 'a' } });
    expect(onvalue).toHaveBeenLastCalledWith('a');
  });

  it('labels a Textarea and passes rows through', () => {
    render(FieldFixture, { props: { kind: 'textarea', label: 'Expression' } });
    const textarea = screen.getByLabelText('Expression');
    expect(textarea.tagName).toBe('TEXTAREA');
    expect(textarea).toHaveAttribute('rows', '4');
  });

  it('works standalone with explicit invalid/size and a caller description', () => {
    render(Input, {
      props: { size: 'md', invalid: true, 'aria-describedby': 'outside-help', 'aria-label': 'Search' }
    });
    const input = screen.getByRole('textbox', { name: 'Search' });
    expect(input).toHaveClass('ui-input--md', 'is-invalid');
    expect(input).toHaveAttribute('aria-describedby', 'outside-help');
    expect(input).toHaveAttribute('type', 'text');
  });
});
