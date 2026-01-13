<h1>Mapping Studio (WIP)</h1>
<p>
  This app will provide a non-developer-friendly interface for HL7 mapping and validation, backed by strictly typed
  contracts (GraphQL + OpenAPI) enforced in CI.
</p>

<div class="card">
  <h2>Next</h2>
  <ul>
    <li>Upload sample HL7 messages</li>
    <li>Preview canonical event extraction via <code>parsePreview</code></li>
    <li>Generate Source Profile config from UI edits</li>
  </ul>
</div>

<style>
  h1 {
    color: #f9fafb;
    margin: 0 0 10px;
  }
  p,
  li {
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
  }
  code {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.95em;
    color: #e5e7eb;
  }
  .card {
    margin-top: 18px;
    padding: 16px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
  }
  .card h2 {
    color: #f3f4f6;
    margin: 0 0 8px;
    font-size: 1.05rem;
  }
</style>

