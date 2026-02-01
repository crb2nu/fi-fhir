/**
 * Shared types for UI components.
 */

/**
 * Tab item definition for the Tabs component.
 */
export type TabItem = {
  key: string;
  label: string;
  disabled?: boolean;
  count?: number;
  icon?: string;
};
