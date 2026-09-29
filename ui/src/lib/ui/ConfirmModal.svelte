<script lang="ts">
  /**
   * ConfirmModal — a confirmation on the `Dialog` primitive: the question as
   * the title, one sentence as its description, optional content, and
   * Cancel / confirm. Escape, the backdrop and the close button cancel,
   * except while `loading`.
   *
   * Legacy-syntax API kept for its callers: `bind:open`, `on:confirm`,
   * `on:cancel`, and a default slot for extra fields.
   */

  import { createEventDispatcher } from 'svelte';
  import { Button, Dialog } from '$lib/ui/primitives';

  export let open = false;
  export let title = 'Confirm';
  export let message = 'Are you sure?';
  export let confirmText = 'Confirm';
  export let cancelText = 'Cancel';
  export let variant: 'primary' | 'danger' = 'primary';
  export let loading = false;
  export let confirmDisabled = false;
  export let closeOnConfirm = true;

  const dispatch = createEventDispatcher<{
    confirm: void;
    cancel: void;
  }>();

  function handleConfirm() {
    if (loading || confirmDisabled) return;
    dispatch('confirm');
    if (closeOnConfirm) {
      open = false;
    }
  }

  function handleCancel() {
    if (loading) return;
    dispatch('cancel');
    open = false;
  }
</script>

<Dialog
  {open}
  {title}
  description={message}
  size="sm"
  dismissible={!loading}
  onclose={handleCancel}
>
  <slot />
  {#snippet footer()}
    <Button variant="ghost" size="md" onclick={handleCancel} disabled={loading}>
      {cancelText}
    </Button>
    <Button {variant} size="md" onclick={handleConfirm} {loading} disabled={confirmDisabled}>
      {confirmText}
    </Button>
  {/snippet}
</Dialog>
