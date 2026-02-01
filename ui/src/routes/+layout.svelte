<script lang="ts">
  import { resolve } from '$app/paths';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  const nav = [
    { href: '/', label: 'Home' },
    { href: '/hl7', label: 'HL7 Mapping' },
    { href: '/profiles', label: 'Profiles' }
  ] as const;
</script>

<ToastContainer />

<div class="app">
  <header class="header">
    <div class="brand">fi-fhir</div>
    <nav class="nav">
      {#each nav as item (item.href)}
        <a class="nav-link" href={resolve(item.href)}>{item.label}</a>
      {/each}
    </nav>
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
  }

  .nav {
    display: flex;
    gap: var(--space-3);
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
  }

  .nav-link:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
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

    .nav-link {
      padding: var(--space-2);
      font-size: var(--text-xs);
    }

    .main {
      padding: var(--space-4);
    }
  }
</style>
