<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import CommandPalette, { type PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import { initTheme } from '$lib/theme/theme';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  const nav = [
    { href: '/', label: 'Home' },
    { href: '/events', label: 'Events' },
    { href: '/hl7', label: 'HL7 Mapping' },
    { href: '/profiles', label: 'Profiles' },
    { href: '/terminology', label: 'Terminology' },
    { href: '/workflows', label: 'Workflows' }
  ] as const;

  let paletteOpen = false;
  let shortcutLabel = 'Ctrl+K';

  function isActive(href: string, pathname: string): boolean {
    if (href === '/') return pathname === '/';
    return pathname === href || pathname.startsWith(href + '/');
  }

  function isEditableTarget(target: EventTarget | null): boolean {
    const el = target as HTMLElement | null;
    if (!el) return false;
    const tag = el.tagName;
    return tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || el.isContentEditable;
  }

  function isHL7Route(pathname: string): boolean {
    return pathname.startsWith('/hl7');
  }

  function openPalette(): void {
    if (isHL7Route($page.url.pathname)) return;
    paletteOpen = true;
  }

  const paletteCommands: PaletteCommand[] = nav.map((item) => ({
    id: `nav:${item.href}`,
    label: `Go to ${item.label}`,
    hint: resolve(item.href),
    keywords: ['navigate', 'route', item.label.toLowerCase()],
    run: () => goto(resolve(item.href))
  }));

  onMount(() => {
    initTheme();
    shortcutLabel = navigator.platform.toUpperCase().includes('MAC') ? 'Cmd+K' : 'Ctrl+K';

    const onKeydown = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (paletteOpen) return;
      if (isEditableTarget(e.target)) return;
      // HL7 page has a richer page-specific command palette.
      if (isHL7Route($page.url.pathname)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        openPalette();
      }
    };

    window.addEventListener('keydown', onKeydown);
    return () => window.removeEventListener('keydown', onKeydown);
  });
</script>

<ToastContainer />
<CommandPalette bind:open={paletteOpen} title="Navigate" commands={paletteCommands} />

<div class="app">
  <header class="header bg-glass">
    <a class="brand text-gradient" href={resolve('/')}>fi-fhir</a>
    <nav class="nav">
      {#each nav as item (item.href)}
        {@const active = isActive(item.href, $page.url.pathname)}
        <a
          class="nav-link"
          class:active
          aria-current={active ? 'page' : undefined}
          href={resolve(item.href)}
        >
          {item.label}
        </a>
      {/each}
    </nav>
    <div class="actions">
      {#if !isHL7Route($page.url.pathname)}
        <button
          type="button"
          class="command-link"
          aria-label="Open navigation commands"
          title={`Open commands (${shortcutLabel})`}
          on:click={openPalette}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.4-3.4" />
          </svg>
          <span class="command-text">Commands</span>
          <span class="command-kbd">{shortcutLabel}</span>
        </button>
      {/if}
      <ThemeToggle />
    </div>
  </header>

  <main class="main">
    <slot />
  </main>
</div>

<style>
  .app {
    min-height: 100vh;
    font-family: var(--font-sans);
    color: var(--color-text-primary);
    background: var(--color-bg-base);
  }

  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border-default);
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
  }

  .brand {
    font-family: var(--font-heading);
    font-weight: 800;
    font-size: var(--text-xl);
    letter-spacing: var(--tracking-tight);
    text-decoration: none;
    transition: var(--transition-transform);
  }

  .brand:hover {
    transform: scale(1.02);
  }

  .nav {
    display: flex;
    gap: var(--space-3);
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
  }

  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .nav-link {
    color: var(--color-text-secondary);
    text-decoration: none;
    padding: var(--space-2) var(--space-4);
    border-radius: var(--radius-full);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    transition: var(--transition-all);
    flex: 0 0 auto;
    border: 1px solid transparent;
  }

  .command-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--color-text-secondary);
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    transition: var(--transition-all);
  }

  .command-link svg {
    width: 14px;
    height: 14px;
    flex: 0 0 auto;
  }

  .command-link:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .command-link:focus-visible {
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .command-kbd {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    padding: 2px 6px;
    line-height: 1.2;
  }

  .nav-link:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .nav-link.active {
    color: var(--color-primary);
    background: var(--color-primary-muted);
    border-color: var(--color-primary-border);
    box-shadow: 0 2px 10px rgba(99, 102, 241, 0.1);
  }

  .nav-link:focus-visible {
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .main {
    padding: var(--space-6) var(--space-5);
    max-width: 1100px;
    margin: 0 auto;
  }

  @media (max-width: 640px) {
    .header {
      padding: var(--space-3) var(--space-4);
    }

    .nav {
      gap: var(--space-2);
    }

    .actions {
      gap: var(--space-1);
    }

    .nav-link {
      padding: var(--space-2);
      font-size: var(--text-xs);
    }

    .command-link {
      padding: var(--space-2);
      font-size: var(--text-xs);
    }

    .command-text,
    .command-kbd {
      display: none;
    }

    .main {
      padding: var(--space-4);
    }
  }
</style>
