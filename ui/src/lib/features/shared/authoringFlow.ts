export type FlowAction = {
  label: string;
  href?: string;
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
