<script lang="ts">
  // Connection bar of a Replay session (docs/02 §3.7, F-REP-2). Edits are saved when the field
  // is committed (blur, Enter, change); the local value stays so a rejected edit can be fixed.
  import { untrack } from 'svelte';
  import type { ReplayConnectionInput, ReplayOptionsInput } from '$lib/api/generated/graphql';
  import type { EnvironmentStore } from '$lib/stores/environment.svelte';
  import type { Connection, Options } from './session.svelte';

  interface Props {
    connection: Connection;
    options: Options;
    environment: EnvironmentStore;
    onconnection: (connection: ReplayConnectionInput) => void;
    onoptions: (options: ReplayOptionsInput) => void;
  }

  let { connection, options, environment, onconnection, onoptions }: Props = $props();

  // Mounted per session (the editor is keyed by session id), so these start from the props.
  let tls = $state(untrack(() => connection.tls));
  let host = $state(untrack(() => connection.host));
  let port = $state(String(untrack(() => connection.port)));
  let sni = $state(untrack(() => connection.sni ?? ''));
  let followRedirects = $state(untrack(() => options.followRedirects));
  let updateContentLength = $state(untrack(() => options.updateContentLength));
  let timeoutMs = $state(String(untrack(() => options.timeoutMs)));
  let applyMatchReplace = $state(untrack(() => options.applyMatchReplace));
  let showOptions = $state(false);

  const field =
    'rounded-token border border-line bg-subtle px-1.5 py-0.5 text-xs focus:outline-none';

  /** Skips an invalid port or an empty host; the server validates the rest. */
  function commitConnection() {
    const p = Number(port);
    if (!host.trim() || !Number.isInteger(p) || p < 1 || p > 65535) return;
    onconnection({ host: host.trim(), port: p, tls, sni: sni.trim() || null });
  }

  function commitTimeout() {
    const ms = Number(timeoutMs);
    if (!Number.isInteger(ms) || ms < 1000 || ms > 300000) return;
    onoptions({ timeoutMs: ms });
  }

  const projectEnvs = $derived(environment.environments.filter((e) => !e.isGlobal));
</script>

<div
  class="flex flex-wrap items-center gap-1.5 border-b border-line bg-elevated px-2 py-1 text-xs"
  role="group"
  aria-label="Connection"
>
  <select
    aria-label="Scheme"
    class={field}
    value={tls ? 'https' : 'http'}
    onchange={(e) => {
      tls = e.currentTarget.value === 'https';
      commitConnection();
    }}
  >
    <option value="https">https</option>
    <option value="http">http</option>
  </select>
  <input aria-label="Host" class="{field} w-48" bind:value={host} onchange={commitConnection} />
  <input
    aria-label="Port"
    inputmode="numeric"
    class="{field} w-16"
    bind:value={port}
    onchange={commitConnection}
  />
  <input
    aria-label="SNI override"
    placeholder="SNI: host"
    class="{field} w-36"
    bind:value={sni}
    onchange={commitConnection}
  />
  <label class="flex items-center gap-1">
    <input
      type="checkbox"
      checked={applyMatchReplace}
      onchange={(e) => {
        applyMatchReplace = e.currentTarget.checked;
        onoptions({ applyMatchReplace });
      }}
    />
    M&amp;R
  </label>

  <label class="ml-auto flex items-center gap-1">
    env:
    <select
      aria-label="Environment"
      class={field}
      value={environment.selected?.id ?? ''}
      onchange={(e) => void environment.select(e.currentTarget.value || null)}
    >
      <option value="">No environment</option>
      {#each projectEnvs as env (env.id)}
        <option value={env.id}>{env.name}</option>
      {/each}
    </select>
  </label>

  <button
    type="button"
    class="rounded-token px-1.5 py-0.5 hover:bg-hover"
    aria-expanded={showOptions}
    aria-controls="replay-options"
    onclick={() => (showOptions = !showOptions)}>⚙</button
  >
</div>

{#if showOptions}
  <div
    id="replay-options"
    role="group"
    aria-label="Send options"
    class="flex flex-wrap items-center gap-3 border-b border-line bg-subtle px-2 py-1 text-xs"
  >
    <label class="flex items-center gap-1">
      <input
        type="checkbox"
        checked={followRedirects}
        onchange={(e) => {
          followRedirects = e.currentTarget.checked;
          onoptions({ followRedirects });
        }}
      />
      Follow redirects
    </label>
    <label class="flex items-center gap-1">
      <input
        type="checkbox"
        checked={updateContentLength}
        onchange={(e) => {
          updateContentLength = e.currentTarget.checked;
          onoptions({ updateContentLength });
        }}
      />
      Update Content-Length
    </label>
    <label class="flex items-center gap-1">
      Timeout
      <input
        aria-label="Timeout (ms)"
        inputmode="numeric"
        class="{field} w-20"
        bind:value={timeoutMs}
        onchange={commitTimeout}
      />
      ms
    </label>
  </div>
{/if}
