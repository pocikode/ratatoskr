<script lang="ts">
  // Placeholder (T-036): T-108 replaces the body with the live before/after diff.
  import type { MatchReplaceRuleInput } from '$lib/api/generated/graphql';

  interface Props {
    /** The editor's current draft; null when no rule is open. */
    rule: MatchReplaceRuleInput | null;
  }

  let { rule }: Props = $props();
</script>

<section aria-label="Tester" class="flex h-full flex-col gap-2 p-3">
  <h2 class="text-xs tracking-wide text-muted uppercase">Tester</h2>
  <p class="text-xs text-muted">
    {rule ? 'The tester arrives with a later task.' : 'Open a rule to test it.'}
  </p>
</section>
