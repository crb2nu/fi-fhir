<script lang="ts">
  const nav = [
    { href: '/', label: 'Home' },
    { href: '/hl7', label: 'HL7 Mapping' }
  ] as const;
</script>

<div class="app">
  <header class="header">
    <div class="brand">fi-fhir</div>
    <nav class="nav">
      {#each nav as item}
        <a class="nav-link" href={item.href}>{item.label}</a>
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
    font-family: system-ui, -apple-system, Segoe UI, Roboto, sans-serif;
    color: #111827;
    background: #0b1220;
  }
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 20px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(2, 6, 23, 0.65);
    backdrop-filter: blur(10px);
    position: sticky;
    top: 0;
  }
  .brand {
    color: #e5e7eb;
    font-weight: 700;
    letter-spacing: 0.2px;
  }
  .nav {
    display: flex;
    gap: 14px;
  }
  .nav-link {
    color: rgba(229, 231, 235, 0.85);
    text-decoration: none;
    padding: 6px 10px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.04);
  }
  .nav-link:hover {
    background: rgba(255, 255, 255, 0.08);
  }
  .main {
    padding: 28px 20px;
    max-width: 1100px;
    margin: 0 auto;
  }
</style>

