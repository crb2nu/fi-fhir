/**
 * Tests for the ConfidenceBadge component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ConfidenceBadge from './ConfidenceBadge.svelte';

describe('ConfidenceBadge', () => {
  describe('percentage display', () => {
    it('should display percentage when showPercent is true', () => {
      render(ConfidenceBadge, { props: { confidence: 0.85 } });

      expect(screen.getByText('85%')).toBeInTheDocument();
    });

    it('should round percentage to nearest integer', () => {
      render(ConfidenceBadge, { props: { confidence: 0.856 } });

      expect(screen.getByText('86%')).toBeInTheDocument();
    });

    it('should not display percentage when showPercent is false', () => {
      render(ConfidenceBadge, { props: { confidence: 0.85, showPercent: false } });

      expect(screen.queryByText('85%')).not.toBeInTheDocument();
    });
  });

  describe('confidence levels', () => {
    it('should have "high" level for confidence >= 0.9', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.95 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('high');
    });

    it('should have "high" level for confidence = 0.9', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.9 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('high');
    });

    it('should have "medium" level for confidence 0.7-0.89', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.75 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('medium');
    });

    it('should have "medium" level for confidence = 0.7', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.7 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('medium');
    });

    it('should have "low" level for confidence 0.5-0.69', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.55 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('low');
    });

    it('should have "low" level for confidence = 0.5', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.5 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('low');
    });

    it('should have "very-low" level for confidence < 0.5', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.3 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('very-low');
    });

    it('should have "very-low" level for confidence = 0', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('very-low');
    });
  });

  describe('progress bar', () => {
    it('should render progress bar', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.75 } });

      expect(container.querySelector('.bar')).toBeInTheDocument();
      expect(container.querySelector('.fill')).toBeInTheDocument();
    });

    it('should set fill width based on confidence', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.65 } });

      const fill = container.querySelector('.fill');
      expect(fill).toHaveStyle({ width: '65%' });
    });

    it('should set fill width to 100% for confidence = 1', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 1 } });

      const fill = container.querySelector('.fill');
      expect(fill).toHaveStyle({ width: '100%' });
    });

    it('should set fill width to 0% for confidence = 0', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0 } });

      const fill = container.querySelector('.fill');
      expect(fill).toHaveStyle({ width: '0%' });
    });
  });

  describe('tooltip', () => {
    it('should have title attribute with confidence info', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.85 } });

      const badge = container.querySelector('.confidence-badge');
      expect(badge).toHaveAttribute('title', '85% confidence (Medium)');
    });

    it('should show "High" label for high confidence', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.95 } });

      const badge = container.querySelector('.confidence-badge');
      expect(badge).toHaveAttribute('title', '95% confidence (High)');
    });

    it('should show "Low" label for low confidence', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.55 } });

      const badge = container.querySelector('.confidence-badge');
      expect(badge).toHaveAttribute('title', '55% confidence (Low)');
    });

    it('should show "Very Low" label for very low confidence', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.3 } });

      const badge = container.querySelector('.confidence-badge');
      expect(badge).toHaveAttribute('title', '30% confidence (Very Low)');
    });
  });

  describe('sizes', () => {
    it('should not have sm class by default', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.75 } });

      expect(container.querySelector('.confidence-badge')).not.toHaveClass('sm');
    });

    it('should have sm class when size is sm', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.75, size: 'sm' } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('sm');
    });
  });

  describe('edge cases', () => {
    it('should handle confidence slightly below threshold', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.899 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('medium');
    });

    it('should handle confidence slightly above threshold', () => {
      const { container } = render(ConfidenceBadge, { props: { confidence: 0.901 } });

      expect(container.querySelector('.confidence-badge')).toHaveClass('high');
    });
  });
});
