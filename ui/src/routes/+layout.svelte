<script lang="ts">
  import { resolve } from '$app/paths';
  import { page } from '$app/stores';
  import { onMount } from 'svelte';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import { initTheme } from '$lib/theme/theme';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  const nav = [
    { href: '/', label: 'Home' },
    { href: '/hl7', label: 'HL7 Mapping' },
    { href: '/profiles', label: 'Profiles' },
    { href: '/terminology', label: 'Terminology' },
    { href: '/workflows', label: 'Workflows' }
  ] as const;

  function isActive(href: string, pathname: string): boolean {
    if (href === '/') return pathname === '/';
    return pathname === href || pathname.startsWith(href + '/');
  }

  onMount(() => {
    initTheme();
  });
</script>

<ToastContainer />

<div class="app">
  <header class="header">
    <a class="brand" href={resolve('/')}>fi-fhir</a>
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
    background: var(--color-bg-overlay);
    backdrop-filter: blur(12px);
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
  }

  .brand {
    color: var(--color-text-primary);
    font-weight: var(--font-bold);
    font-size: var(--text-lg);
    letter-spacing: var(--tracking-tight);
    text-decoration: none;
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
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    transition: var(--transition-all);
    flex: 0 0 auto;
  }

  .nav-link:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .nav-link.active {
    color: var(--color-text-primary);
    background: rgba(59, 130, 246, 0.14);
    border-color: rgba(59, 130, 246, 0.38);
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

    .main {
      padding: var(--space-4);
    }
  }
</style>
