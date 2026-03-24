/**
 * Tests for the StepControls component.
 */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import StepControls from './StepControls.svelte';

describe('StepControls', () => {
  describe('rendering', () => {
    it('should render all 5 control buttons', () => {
      render(StepControls, { props: { state: 'idle' } });

      expect(screen.getByRole('button', { name: 'Play' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Step Over' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Continue' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Stop' })).toBeInTheDocument();
    });

    it('should render toolbar role', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('toolbar')).toBeInTheDocument();
    });
  });

  describe('idle state', () => {
    it('should enable Play when idle', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('button', { name: 'Play' })).not.toBeDisabled();
    });

    it('should disable Step Over when idle', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('button', { name: 'Step Over' })).toBeDisabled();
    });

    it('should disable Continue when idle', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();
    });

    it('should disable Restart when idle', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('button', { name: 'Restart' })).toBeDisabled();
    });

    it('should disable Stop when idle', () => {
      render(StepControls, { props: { state: 'idle' } });
      expect(screen.getByRole('button', { name: 'Stop' })).toBeDisabled();
    });
  });

  describe('running state', () => {
    it('should disable Play when running', () => {
      render(StepControls, { props: { state: 'running' } });
      expect(screen.getByRole('button', { name: 'Play' })).toBeDisabled();
    });

    it('should enable Stop when running', () => {
      render(StepControls, { props: { state: 'running' } });
      expect(screen.getByRole('button', { name: 'Stop' })).not.toBeDisabled();
    });
  });

  describe('paused state', () => {
    it('should enable Step Over when paused', () => {
      render(StepControls, { props: { state: 'paused' } });
      expect(screen.getByRole('button', { name: 'Step Over' })).not.toBeDisabled();
    });

    it('should enable Continue when paused', () => {
      render(StepControls, { props: { state: 'paused' } });
      expect(screen.getByRole('button', { name: 'Continue' })).not.toBeDisabled();
    });

    it('should enable Restart when paused', () => {
      render(StepControls, { props: { state: 'paused' } });
      expect(screen.getByRole('button', { name: 'Restart' })).not.toBeDisabled();
    });

    it('should enable Stop when paused', () => {
      render(StepControls, { props: { state: 'paused' } });
      expect(screen.getByRole('button', { name: 'Stop' })).not.toBeDisabled();
    });
  });

  describe('completed state', () => {
    it('should enable Restart when completed', () => {
      render(StepControls, { props: { state: 'completed' } });
      expect(screen.getByRole('button', { name: 'Restart' })).not.toBeDisabled();
    });

    it('should disable Stop when completed', () => {
      render(StepControls, { props: { state: 'completed' } });
      expect(screen.getByRole('button', { name: 'Stop' })).toBeDisabled();
    });
  });

  describe('event dispatching', () => {
    it('should call onPlay when Play clicked', async () => {
      const onPlay = vi.fn();
      render(StepControls, { props: { state: 'idle', onPlay } });

      await fireEvent.click(screen.getByRole('button', { name: 'Play' }));
      expect(onPlay).toHaveBeenCalledTimes(1);
    });

    it('should call onStep when Step Over clicked', async () => {
      const onStep = vi.fn();
      render(StepControls, { props: { state: 'paused', onStep } });

      await fireEvent.click(screen.getByRole('button', { name: 'Step Over' }));
      expect(onStep).toHaveBeenCalledTimes(1);
    });

    it('should call onContinue when Continue clicked', async () => {
      const onContinue = vi.fn();
      render(StepControls, { props: { state: 'paused', onContinue } });

      await fireEvent.click(screen.getByRole('button', { name: 'Continue' }));
      expect(onContinue).toHaveBeenCalledTimes(1);
    });

    it('should call onRestart when Restart clicked', async () => {
      const onRestart = vi.fn();
      render(StepControls, { props: { state: 'paused', onRestart } });

      await fireEvent.click(screen.getByRole('button', { name: 'Restart' }));
      expect(onRestart).toHaveBeenCalledTimes(1);
    });

    it('should call onStop when Stop clicked', async () => {
      const onStop = vi.fn();
      render(StepControls, { props: { state: 'paused', onStop } });

      await fireEvent.click(screen.getByRole('button', { name: 'Stop' }));
      expect(onStop).toHaveBeenCalledTimes(1);
    });
  });
});
