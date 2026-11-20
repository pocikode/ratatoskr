import { getApi } from '$lib/api/client';
import { CreateReplaySessionDocument } from '$lib/api/generated/graphql';
import { router } from '$lib/router/router.svelte';

/** Copies a History/Intercept request into a new session and opens it; returns an error message. */
export async function openInReplay(
  requestId: string,
  client = getApi().client,
  navigate: (page: string, ...params: string[]) => void = (...a) => router.navigate(...a),
): Promise<string | null> {
  const r = await client
    .mutation(CreateReplaySessionDocument, { input: { fromRequestId: requestId } })
    .toPromise();
  if (r.error || !r.data) return r.error?.graphQLErrors[0]?.message ?? 'Request failed';
  navigate('replay', 'session', r.data.createReplaySession.id);
  return null;
}
