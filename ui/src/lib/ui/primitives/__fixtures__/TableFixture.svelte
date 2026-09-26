<!-- Test fixture: a sortable, selectable Table the way a page composes it. -->
<script lang="ts">
  import { Badge, Table, Td, Th, Tr, type SortDirection } from '../index';

  interface Row {
    id: string;
    type: string;
    count: number;
  }

  interface Props {
    rows: Row[];
    selectedId?: string | undefined;
    sort?: SortDirection;
    onselect?: (id: string) => void;
    onsort?: () => void;
  }

  let { rows, selectedId, sort = 'none', onselect, onsort }: Props = $props();
</script>

<Table label="Events" data-testid="events-table">
  {#snippet head()}
    <tr>
      <Th {sort} {onsort}>Type</Th>
      <Th>Id</Th>
      <Th numeric>Count</Th>
      <Th>Status</Th>
    </tr>
  {/snippet}
  {#each rows as row (row.id)}
    <Tr
      selectable
      selected={row.id === selectedId}
      onselect={() => onselect?.(row.id)}
      data-testid={`row-${row.id}`}
    >
      <Td value={row.type} />
      <Td mono truncate value={row.id} />
      <Td numeric value={row.count} />
      <Td><Badge tone="success">ok</Badge></Td>
    </Tr>
  {/each}
</Table>
