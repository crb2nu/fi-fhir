import type { IDEAppRoute } from '$lib/ui/ide/types';

export type FlowAction = {
  label: string;
  href?: IDEAppRoute;
  onClick?: () => void | Promise<void>;
  variant?: 'primary' | 'secondary' | 'ghost';
  ariaLabel?: string;
};

export type FlowStep = {
  eyebrow: string;
  title: string;
  description: string;
  status?: string;
  metric?: string;
  actions?: readonly FlowAction[];
};
