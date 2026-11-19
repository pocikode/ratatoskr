<script lang="ts">
  // Placeholder: T-107 owns the session body (connection bar, request/response editors).
  interface Props {
    sessionId: string;
  }

  let { sessionId }: Props = $props();
</script>

<div class="p-4 text-muted" data-session-id={sessionId}>Session {sessionId}</div>
