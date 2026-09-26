<!--
  IconButton — a square Button holding one 16px icon. `label` is required: it
  becomes the accessible name and the tooltip. Use `pressed` for toggles.
-->
<script lang="ts">
  import type { HTMLButtonAttributes } from 'svelte/elements';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import type { ButtonVariant, ControlSize, IconComponent } from './types';

  interface Props extends Omit<HTMLButtonAttributes, 'children'> {
    icon: IconComponent;
    label: string;
    variant?: ButtonVariant;
    size?: ControlSize;
    /** Toggle state; renders aria-pressed. Leave undefined for plain actions. */
    pressed?: boolean | undefined;
    loading?: boolean;
  }

  let {
    icon,
    label,
    variant = 'ghost',
    size = 'sm',
    pressed,
    loading = false,
    title,
    ...rest
  }: Props = $props();
</script>

<Button
  {...rest}
  {variant}
  {size}
  {loading}
  iconOnly
  aria-label={label}
  aria-pressed={pressed === undefined ? undefined : pressed ? 'true' : 'false'}
  title={title ?? label}
>
  {#if !loading}
    <Icon {icon} />
  {/if}
</Button>
