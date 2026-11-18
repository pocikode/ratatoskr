<script lang="ts">
  import { formParams, queryParams, type ParsedMessage } from './parse';

  let { parsed }: { parsed: ParsedMessage } = $props();

  /** A form body can hold millions of params; the table is a sample, not the whole message. */
  const MAX_PARAMS = 500;
  const MAX_FORM_CHARS = 128 * 1024;

  const body = $derived(parsed.body.slice(0, MAX_FORM_CHARS));
  const all = $derived([
    ...queryParams(parsed.startLine).map((p) => ({ ...p, source: 'query' })),
    ...formParams({ ...parsed, body }).map((p) => ({ ...p, source: 'form' })),
  ]);
  const params = $derived(all.slice(0, MAX_PARAMS));
  const more = $derived(all.length - params.length);
</script>

<div class="space-y-3 p-2 text-xs">
  <table class="w-full text-left">
    <caption class="pb-1 text-left text-muted">Headers</caption>
    <tbody>
      <tr class="align-top">
        <th scope="row" class="w-40 py-0.5 pr-2 font-semibold text-muted">Start line</th>
        <td class="font-mono break-all">{parsed.startLine}</td>
      </tr>
      {#each parsed.headers as h, i (i)}
        <tr class="align-top">
          <th scope="row" class="w-40 py-0.5 pr-2 font-semibold text-muted">{h.name}</th>
          <td class="font-mono break-all">{h.value}</td>
        </tr>
      {/each}
    </tbody>
  </table>

  {#if params.length > 0}
    <table class="w-full text-left">
      <caption class="pb-1 text-left text-muted">Params</caption>
      <thead>
        <tr class="text-muted">
          <th scope="col" class="w-16 py-0.5 pr-2">Source</th>
          <th scope="col" class="w-40 py-0.5 pr-2">Name</th>
          <th scope="col" class="py-0.5">Value</th>
        </tr>
      </thead>
      <tbody>
        {#each params as p, i (i)}
          <tr class="align-top">
            <td class="py-0.5 pr-2 text-muted">{p.source}</td>
            <td class="py-0.5 pr-2 font-mono break-all">{p.name}</td>
            <td class="py-0.5 font-mono break-all">{p.value}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if more > 0 || parsed.body.length > MAX_FORM_CHARS}
      <p class="text-muted">
        {#if more > 0}…and {more} more params not shown.{/if}
        {#if parsed.body.length > MAX_FORM_CHARS}
          Only the first {MAX_FORM_CHARS / 1024} KB of the body was parsed.
        {/if}
      </p>
    {/if}
  {/if}
</div>
