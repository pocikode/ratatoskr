<script lang="ts">
  import type { Component } from 'svelte';
  import { DEFAULT_ROUTE, findRoute } from '$lib/router/routes';
  import { router } from '$lib/router/router.svelte';
  import PlaceholderPage from './PlaceholderPage.svelte';

  let Page = $state.raw<Component | null>(null);
  let failed = $state(false);

  // Lazy page load; `cancelled` drops results of superseded navigations.
  $effect(() => {
    const route = router.route;
    let cancelled = false;
    failed = false;
    if (!route) {
      Page = null;
      return;
    }
    route
      .load()
      .then((m) => {
        if (!cancelled) Page = m.default;
      })
      .catch(() => {
        if (!cancelled) failed = true;
      });
    return () => {
      cancelled = true;
    };
  });
</script>

<main id="main" class="h-full overflow-auto p-4" tabindex="-1">
  {#if !router.route}
    <section class="flex h-full flex-col items-center justify-center gap-2 text-center">
      <h1 class="text-lg font-semibold">Page not found</h1>
      <a class="text-accent underline" href={`#/${DEFAULT_ROUTE}`}>
        Go to {findRoute(DEFAULT_ROUTE)?.title}
      </a>
    </section>
  {:else if failed}
    <p role="alert" class="text-danger">Failed to load {router.route.title}.</p>
  {:else if Page}
    {#key router.route.id}
      <Page />
    {/key}
  {:else}
    <PlaceholderPage title={router.route.title} />
  {/if}
</main>
