import { describe, expect, it } from 'vitest';
import { getSidebarContext, getSidebarView, getSidebarViewLinks } from './sidebarContent';

describe('sidebarContent', () => {
  it('maps nested routes to the correct sidebar view', () => {
    expect(getSidebarView('/events/patient-123')).toBe('events');
    expect(getSidebarView('/profiles/draft')).toBe('profiles');
    expect(getSidebarView('/terminology/autoroute')).toBe('terminology');
  });

  it('returns contextual content for the workflows view', () => {
    const context = getSidebarContext('/workflows/monitor');

    expect(context.view).toBe('workflows');
    expect(context.title).toBe('Delivery');
    expect(context.actions).toHaveLength(3);
    expect(context.journey.stage?.label).toBe('Delivery');
    expect(context.journey.nextAction.label).toBe('Continue to Verification');
    expect(context.recent[0]?.label).toBe('Workflow builder');
  });

  it('returns the top-level navigation entries in route order', () => {
    const links = getSidebarViewLinks();

    expect(links.map((link) => link.href)).toEqual([
      '/',
      '/hl7',
      '/profiles',
      '/terminology',
      '/workflows',
      '/events',
      '/operator',
    ]);
  });
});
