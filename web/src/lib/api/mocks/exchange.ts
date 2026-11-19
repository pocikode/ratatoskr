// urql exchange that answers HTTP History, Intercept, scope- and filter-preset operations from a fake backend, so the UI
// works with no server (docs/07-agent-workflow.md: contract-first mocks). It validates
// and runs the real schema with graphql-js, so selections, arguments and errors behave
// as on the wire. Operations touching any other root field are forwarded unchanged.
import { Client, makeErrorResult, makeResult, type Exchange, type Operation } from '@urql/core';
import {
  buildSchema,
  execute,
  Kind,
  subscribe,
  validate,
  type DocumentNode,
  type ExecutionResult,
} from 'graphql';
import { filter, make, merge, mergeMap, pipe, share, takeUntil } from 'wonka';
import sdl from '../generated/schema.graphql?raw';
import { createHistoryMock, type MockHistoryOptions } from './history';
import { createInterceptMock } from './intercept';
import { createPresetsMock } from './presets';
import { createSitemapMock } from './sitemap';

const schema = buildSchema(sdl);

export interface MockExchangeOptions extends MockHistoryOptions {
  /** Interval in ms at which Intercept holds a new request while queuing; 0 (default) disables it. */
  interceptLiveIntervalMs?: number;
}

function rootFields(doc: DocumentNode): string[] {
  return doc.definitions.flatMap((d) =>
    d.kind === Kind.OPERATION_DEFINITION
      ? d.selectionSet.selections.flatMap((s) =>
          s.kind === Kind.FIELD && s.name.value !== '__typename' ? [s.name.value] : [],
        )
      : [],
  );
}

function isIterator(v: unknown): v is AsyncGenerator<ExecutionResult> {
  return typeof v === 'object' && v !== null && Symbol.asyncIterator in v;
}

/**
 * Exchange serving the history, intercept, preset and sitemap contracts ({@link createHistoryMock},
 * {@link createInterceptMock}, {@link createPresetsMock}, {@link createSitemapMock}).
 */
export function mockHistoryExchange(opts: MockExchangeOptions = {}): Exchange {
  const history = createHistoryMock(opts);
  const presets = createPresetsMock().root;
  const root = {
    ...history.root,
    ...presets,
    ...createSitemapMock(history.rows, history.matcher).root,
    ...createInterceptMock(history, (id) => presets.scopePreset({ id }), {
      liveIntervalMs: opts.interceptLiveIntervalMs,
    }).root,
  };
  const handles = (op: Operation) => {
    const fields = rootFields(op.query);
    return fields.length > 0 && fields.every((f) => f in root);
  };

  return ({ forward }) =>
    (ops$) => {
      const shared = pipe(ops$, share);
      const mocked = pipe(
        shared,
        filter((op) => op.kind !== 'teardown' && handles(op)),
        mergeMap((op) =>
          pipe(
            make<ReturnType<typeof makeResult>>((observer) => {
              let iterator: AsyncGenerator<ExecutionResult> | undefined;
              let closed = false;
              const args = {
                schema,
                document: op.query,
                rootValue: root,
                variableValues: op.variables as Record<string, unknown> | undefined,
              };
              void (async () => {
                try {
                  // execute/subscribe skip validation; the server does not. The await also
                  // keeps emission async, after urql has subscribed.
                  const errors = validate(schema, op.query);
                  const out = await (errors.length
                    ? { errors }
                    : op.kind === 'subscription'
                      ? subscribe(args)
                      : execute(args));
                  if (isIterator(out)) {
                    iterator = out;
                    if (closed) await out.return();
                    for await (const result of out) observer.next(makeResult(op, result));
                  } else {
                    observer.next(makeResult(op, out));
                  }
                } catch (e) {
                  observer.next(makeErrorResult(op, e instanceof Error ? e : new Error(String(e))));
                }
                observer.complete();
              })();
              return () => {
                closed = true;
                void iterator?.return(undefined);
              };
            }),
            takeUntil(
              pipe(
                shared,
                filter((t) => t.kind === 'teardown' && t.key === op.key),
              ),
            ),
          ),
        ),
      );
      const rest = forward(
        pipe(
          shared,
          filter((op) => op.kind === 'teardown' || !handles(op)),
        ),
      );
      return merge([mocked, rest]);
    };
}

/** A client that serves only the mocked operations; for tests and stories. */
export function createMockClient(opts: MockExchangeOptions = {}): Client {
  return new Client({ url: '/mock', exchanges: [mockHistoryExchange(opts)] });
}
