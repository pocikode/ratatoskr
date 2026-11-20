/* eslint-disable */
/** Internal type. DO NOT USE DIRECTLY. */
type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
/** Internal type. DO NOT USE DIRECTLY. */
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
/** What a completion inserts. */
export type CompletionKind =
  /** A field such as `host` or `header`. */
  | 'FIELD'
  /** `req`, `resp`, `row`, `ws`, `stream`. */
  | 'NAMESPACE'
  /** An operator such as `eq` or `cont`. */
  | 'OPERATOR'
  /** A filter preset name or alias, after `preset:`. */
  | 'PRESET'
  /** A value: distinct values from the project, booleans, sources, and so on. */
  | 'VALUE';

/**
 * A new session: either copied from a history request (`fromRequestId`) or from
 * scratch (`connection` and `raw`). Giving both or neither is `BAD_INPUT`.
 */
export type CreateReplaySessionInput = {
  /** Collection to add the session to, placed last; null for the Default collection. */
  collectionId?: string | number | null | undefined;
  /** Connection for a session from scratch. */
  connection?: ReplayConnectionInput | null | undefined;
  /**
   * History request to copy: its stored request bytes and connection. `NOT_FOUND`
   * when unknown; `BAD_INPUT` when its stored request was truncated or is larger
   * than 16 MiB (checked before the body is read).
   */
  fromRequestId?: string | number | null | undefined;
  /** Name; null for `METHOD path` (see `ReplaySession.name` for the name rules). */
  name?: string | null | undefined;
  /**
   * Request bytes for a session from scratch. The URL dialog sends
   * `GET <path> HTTP/1.1` with a `Host` header. At most 16 MiB (16777216 bytes)
   * decoded, else `BAD_INPUT`; stored exactly as given.
   */
  raw?: string | null | undefined;
};

/** A variable to store; same rules as `EnvironmentVariable`. */
export type EnvironmentVariableInput = {
  /** Variable name (see `EnvironmentVariable.name`). */
  name: string;
  /** Whether the value is a secret. Default false. */
  secret?: boolean;
  /** Value, at most 1 MiB. */
  value: string;
};

/** A rule's matcher to store; same rules as `MatchReplaceMatcher`. */
export type MatchReplaceMatcherInput = {
  /** How to match. */
  kind: MatchReplaceMatcherKind;
  /**
   * The string, pattern or name; must not be empty for `STRING`, `REGEX`, `NAME`
   * except with `ADD`, which ignores the matcher (any kind and value, stored as
   * given); at most 1 MiB. Dropped (stored empty) for `FULL` unless the operation
   * is `ADD`.
   */
  value?: string;
};

/** How a rule finds what it changes. */
export type MatchReplaceMatcherKind =
  /** The whole section, or every item. `value` is ignored. */
  | 'FULL'
  /**
   * A header name (case-insensitive) or query param name (case-sensitive). Only
   * with `UPDATE_VALUE` and `REMOVE`. `value` must not be empty.
   */
  | 'NAME'
  /**
   * A Go RE2 regular expression; every match. RE2 has no look-around and no
   * backreferences (`(?=…)`, `(?<=…)`, `\1` are invalid). An invalid pattern is
   * `BAD_INPUT` with the RE2 error message. In the term, `$1`, `${name}` expand to
   * capture groups and `$$` is a literal `$`. `value` must not be empty.
   */
  | 'REGEX'
  /** Exact text, case-sensitive; every occurrence. `value` must not be empty. */
  | 'STRING';

/** What a rule does to its section. */
export type MatchReplaceOperation =
  /**
   * Appends one header or param after the last one. The term is `Name: value`
   * for headers (`BAD_INPUT` when the name is not an HTTP token or `:` is missing)
   * and `name=value` (or `name`) for params, appended as written. The matcher is
   * ignored.
   */
  | 'ADD'
  /**
   * Removes headers or params: `NAME` by name, `STRING`/`REGEX` when they match
   * the item text (`Name: value` or `name=value`), `FULL` removes all. The term is
   * ignored. Matchers: all four.
   */
  | 'REMOVE'
  /**
   * Replaces matched text in the method or status code token (`FULL` replaces the
   * token). Only for `REQUEST_METHOD` and `RESPONSE_STATUS_CODE`.
   * Matchers: `FULL`, `STRING`, `REGEX`.
   */
  | 'UPDATE'
  /**
   * Replaces matched text in the section text: the start line, the path, the raw
   * query string, each header line on its own, or the body. With `FULL` the term
   * replaces the whole text (every header line for header sections).
   * Matchers: `FULL`, `STRING`, `REGEX`.
   */
  | 'UPDATE_RAW'
  /**
   * Changes header or query param values. `NAME` selects items by name and the
   * term replaces the whole value; `STRING`/`REGEX` replace inside every item's
   * value; `FULL` replaces every item's value. Matchers: all four.
   */
  | 'UPDATE_VALUE';

/**
 * A whole rule. Validation (`BAD_INPUT` unless noted, nothing changes on error):
 * name 1-200 characters after trimming without control characters; `sources`
 * empty; section x operation not valid; matcher kind not allowed for the
 * operation; empty matcher value for `STRING`/`REGEX`/`NAME` (not checked for
 * `ADD`, which ignores the matcher); invalid regex (the
 * message carries the RE2 error); `ADD` header term not `Name: value`; term or
 * matcher value over 1 MiB; condition over 64 KiB. An invalid `condition` is
 * `BAD_QUERY` with `extensions.span`. Saving (not testing) also checks the
 * project budget: the UTF-8 sum of all rules' conditions, matcher values and
 * terms is at most 3 MiB.
 */
export type MatchReplaceRuleInput = {
  /** HTTPQL condition, at most 64 KiB; empty (the default) means always. */
  condition?: string;
  /** Whether the rule applies. Default true. */
  enabled?: boolean;
  /** How the rule finds what it changes. */
  matcher: MatchReplaceMatcherInput;
  /** Display name (see `MatchReplaceRule.name`). */
  name: string;
  /** What the rule does. */
  operation: MatchReplaceOperation;
  /** The part of the message the rule works on. */
  section: MatchReplaceSection;
  /** Traffic the rule applies to; at least one, duplicates dropped. */
  sources: Array<MatchReplaceSource>;
  /** Replacement text, at most 1 MiB. Default empty. */
  term?: string;
};

/**
 * The part of a message a rule works on. `REQUEST_*` sections apply to requests,
 * `RESPONSE_*` sections to responses. Valid operations per section:
 * first line, path and body take `UPDATE_RAW`; method and status code take
 * `UPDATE`; query and header take `UPDATE_RAW`, `UPDATE_VALUE`, `ADD`, `REMOVE`.
 * WebSocket sections arrive with M4 (T-053).
 */
export type MatchReplaceSection =
  /** The request body, after removing one `Content-Encoding` (see `MatchReplaceRule`). `UPDATE_RAW` only. */
  | 'REQUEST_BODY'
  /** The request line (`GET /a?b=c HTTP/1.1`, without CRLF). `UPDATE_RAW` only. */
  | 'REQUEST_FIRST_LINE'
  /**
   * Request headers. `UPDATE_RAW` works on each header line (`Name: value`) on its
   * own; `UPDATE_VALUE`, `ADD`, `REMOVE` on headers (names case-insensitive).
   */
  | 'REQUEST_HEADER'
  /** The method token. `UPDATE` only. */
  | 'REQUEST_METHOD'
  /** The path of the request target, without `?` and query. `UPDATE_RAW` only. */
  | 'REQUEST_PATH'
  /**
   * The query string of the request target. `UPDATE_RAW` works on the raw string
   * after `?`; `UPDATE_VALUE`, `ADD`, `REMOVE` work on its `&`-separated params
   * (names case-sensitive).
   */
  | 'REQUEST_QUERY'
  /** The response body, after removing one `Content-Encoding`. `UPDATE_RAW` only. */
  | 'RESPONSE_BODY'
  /** The status line (`HTTP/1.1 200 OK`, without CRLF). `UPDATE_RAW` only. */
  | 'RESPONSE_FIRST_LINE'
  /** Response headers; same operations and rules as `REQUEST_HEADER`. */
  | 'RESPONSE_HEADER'
  /**
   * The three-digit status code. `UPDATE` only; a result that is not three digits
   * skips the rule (logged).
   */
  | 'RESPONSE_STATUS_CODE';

/** Which traffic a rule applies to. */
export type MatchReplaceSource =
  /** Automate requests (stored now, applied once Automate lands). */
  | 'AUTOMATE'
  /** A message forwarded from the Intercept editor, after the user's edit. */
  | 'INTERCEPT'
  /** Plugin requests (stored now, applied once plugins land). */
  | 'PLUGIN'
  /** Traffic captured by the proxy, before Intercept holds it. */
  | 'PROXY'
  /** Replay sends whose session has "Apply Match & Replace" on. */
  | 'REPLAY'
  /** Workflow requests (stored now, applied once workflows land). */
  | 'WORKFLOW';

/** What happened to a project in a `projectChanged` event. */
export type ProjectChangeKind =
  /** A project was created (including by duplication or as a temporary project). */
  | 'CREATED'
  /** A project was deleted. `project` is null. */
  | 'DELETED'
  /**
   * A different project was selected. Clients must drop all project-scoped state
   * and re-run their queries and subscriptions.
   */
  | 'SELECTED'
  /** A project's fields changed: name, temporary or default flag. */
  | 'UPDATED';

/**
 * A connection to store or send to. Errors: `BAD_INPUT` for an empty `host` or
 * one containing whitespace or control characters, a `port` outside 1-65535, or
 * an empty `sni` (use null for no override).
 */
export type ReplayConnectionInput = {
  /** Host name or IP address to dial. */
  host: string;
  /** TCP port, 1-65535. */
  port: number;
  /** SNI override; null (or omitted) for none. */
  sni?: string | null | undefined;
  /** Whether to speak TLS. */
  tls: boolean;
};

/**
 * Changes to `ReplayOptions`. Omitted or null fields keep their value. `BAD_INPUT`
 * for `timeoutMs` outside 1000-300000; nothing changes then.
 */
export type ReplayOptionsInput = {
  /** See `ReplayOptions.applyMatchReplace`. */
  applyMatchReplace?: boolean | null | undefined;
  /** See `ReplayOptions.followRedirects`. */
  followRedirects?: boolean | null | undefined;
  /** See `ReplayOptions.timeoutMs`; 1000-300000. */
  timeoutMs?: number | null | undefined;
  /** See `ReplayOptions.updateContentLength`. */
  updateContentLength?: boolean | null | undefined;
};

/**
 * Row colour of a request (F-HIST-1). `null` wherever a colour is optional means
 * no colour. The UI picks the actual shades.
 */
export type RequestColor =
  | 'BLUE'
  | 'CYAN'
  | 'GREEN'
  | 'ORANGE'
  | 'PINK'
  | 'PURPLE'
  | 'RED'
  | 'YELLOW';

/**
 * Order of a `requests` connection. Ties are always broken by ID in the same
 * direction, so the order is total and cursors stay stable.
 */
export type RequestSort = {
  direction?: SortDirection;
  field: RequestSortField;
};

/** Columns the history table can sort by (F-HIST-1). */
export type RequestSortField =
  /** Colour in `RequestColor` declaration order; no colour sorts as lowest. */
  | 'COLOR'
  | 'CREATED_AT'
  /** Edited records (`editedFrom` set) sort after unedited ones. */
  | 'EDITED'
  | 'EXT'
  | 'HOST'
  | 'ID'
  | 'METHOD'
  /** Note text; no note sorts as lowest. */
  | 'NOTE'
  | 'PATH'
  | 'PORT'
  | 'QUERY'
  /** Response length in bytes; requests without a response sort as lowest. */
  | 'RESPONSE_LENGTH'
  /** Roundtrip in ms; requests without a response sort as lowest. */
  | 'ROUNDTRIP'
  | 'SOURCE'
  /** Response status code; requests without a response sort as lowest. */
  | 'STATUS'
  | 'TLS';

/** What produced a request record. Same values as the HTTPQL `source:` clause. */
export type RequestSource =
  /** Sent by an Automate attack. */
  | 'AUTOMATE'
  /**
   * The edited record of a proxied exchange the user changed in Intercept
   * (F-HIST-4). The original record of the pair keeps `PROXY`.
   */
  | 'INTERCEPT'
  /** Sent by a plugin. */
  | 'PLUGIN'
  /** Captured by the proxy. */
  | 'PROXY'
  /** Sent from a Replay session. */
  | 'REPLAY'
  /** Sent by a workflow. */
  | 'WORKFLOW';

/** What to send. The values are first saved on the session, as `updateReplaySession` would. */
export type SendReplaySessionInput = {
  /** Saved as the session's connection. */
  connection: ReplayConnectionInput;
  /** Merged into the session's options; null keeps them. */
  options?: ReplayOptionsInput | null | undefined;
  /** The editor's bytes; saved as the draft. At most 16 MiB (16777216 bytes) decoded, else `BAD_INPUT`. */
  raw: string;
};

/** What a sitemap node stands for; clients pick the icon from it. */
export type SitemapNodeKind =
  /** A path segment that has child path segments. */
  | 'DIRECTORY'
  /** A path segment without child path segments (it may have `QUERY` children). */
  | 'FILE'
  /** One `(tls, host, port)` target. Always a root. */
  | 'HOST'
  /**
   * Requests whose path ends at the parent path node and whose query string has
   * this exact set of parameter names (values ignored). Always a leaf.
   */
  | 'QUERY';

/** Sort direction. */
export type SortDirection =
  | 'ASC'
  | 'DESC';

/** Changes to a session. Omitted or null fields keep their value; nothing changes when any field is invalid. */
export type UpdateReplaySessionInput = {
  /** Replaces the whole connection. */
  connection?: ReplayConnectionInput | null | undefined;
  /** Replaces the draft bytes: at most 16 MiB (16777216 bytes) decoded, else `BAD_INPUT`; stored exactly as given. */
  draft?: string | null | undefined;
  /** New name; see `ReplaySession.name` for the name rules. */
  name?: string | null | undefined;
  /** Merged into the current options. */
  options?: ReplayOptionsInput | null | undefined;
};

export type ServerInfoQueryVariables = Exact<{ [key: string]: never; }>;


export type ServerInfoQuery = { serverInfo: { version: string, commit: string, goVersion: string, startedAt: string, authRequired: boolean } };

export type PingSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type PingSubscription = { ping: { seq: string, at: string } };

export type EnvironmentFieldsFragment = { id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> };

export type EnvironmentsQueryVariables = Exact<{ [key: string]: never; }>;


export type EnvironmentsQuery = { environments: Array<{ id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> }> };

export type CreateEnvironmentMutationVariables = Exact<{
  name: string;
}>;


export type CreateEnvironmentMutation = { createEnvironment: { id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> } };

export type RenameEnvironmentMutationVariables = Exact<{
  id: string | number;
  name: string;
}>;


export type RenameEnvironmentMutation = { renameEnvironment: { id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> } };

export type DeleteEnvironmentMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteEnvironmentMutation = { deleteEnvironment: string };

export type SetEnvironmentVariablesMutationVariables = Exact<{
  id: string | number;
  variables: Array<EnvironmentVariableInput> | EnvironmentVariableInput;
}>;


export type SetEnvironmentVariablesMutation = { setEnvironmentVariables: { id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> } };

export type SelectEnvironmentMutationVariables = Exact<{
  id?: string | number | null | undefined;
}>;


export type SelectEnvironmentMutation = { selectEnvironment: { id: string, name: string, isGlobal: boolean, selected: boolean, variables: Array<{ name: string, value: string, secret: boolean }> } | null };

export type FilterPresetFieldsFragment = { id: string, name: string, alias: string | null, query: string };

export type FilterPresetsQueryVariables = Exact<{ [key: string]: never; }>;


export type FilterPresetsQuery = { filterPresets: Array<{ id: string, name: string, alias: string | null, query: string }> };

export type FilterMatchCountQueryVariables = Exact<{
  query: string;
}>;


export type FilterMatchCountQuery = { requests: { totalCount: number } };

export type CreateFilterPresetMutationVariables = Exact<{
  name: string;
  alias?: string | null | undefined;
  query: string;
}>;


export type CreateFilterPresetMutation = { createFilterPreset: { id: string, name: string, alias: string | null, query: string } };

export type UpdateFilterPresetMutationVariables = Exact<{
  id: string | number;
  name: string;
  alias?: string | null | undefined;
  query: string;
}>;


export type UpdateFilterPresetMutation = { updateFilterPreset: { id: string, name: string, alias: string | null, query: string } };

export type DeleteFilterPresetMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteFilterPresetMutation = { deleteFilterPreset: string };

export type ReorderFilterPresetsMutationVariables = Exact<{
  ids: Array<string | number> | string | number;
}>;


export type ReorderFilterPresetsMutation = { reorderFilterPresets: Array<{ id: string, name: string, alias: string | null, query: string }> };

export type ExportFilterPresetsQueryVariables = Exact<{ [key: string]: never; }>;


export type ExportFilterPresetsQuery = { exportFilterPresets: string };

export type ImportFilterPresetsMutationVariables = Exact<{
  json: string;
}>;


export type ImportFilterPresetsMutation = { importFilterPresets: number };

export type HistoryRowFragment = { id: string, source: RequestSource, host: string, port: number, tls: boolean, method: string, path: string, query: string, ext: string, length: string, createdAt: string, color: RequestColor | null, note: string | null, isDropped: boolean, isTruncated: boolean, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { code: number, length: string, roundtripMs: number, createdAt: string } | null };

export type HistoryMessageFragment = { id: string, head: string, bodyLength: string, body: string, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { head: string, bodyLength: string, body: string } | null };

export type HistoryRequestsQueryVariables = Exact<{
  query?: string | null | undefined;
  scopeId?: string | number | null | undefined;
  first: number;
  after?: string | null | undefined;
  sort?: RequestSort | null | undefined;
}>;


export type HistoryRequestsQuery = { requests: { totalCount: number, pageInfo: { hasNextPage: boolean, endCursor: string | null }, edges: Array<{ cursor: string, node: { id: string, source: RequestSource, host: string, port: number, tls: boolean, method: string, path: string, query: string, ext: string, length: string, createdAt: string, color: RequestColor | null, note: string | null, isDropped: boolean, isTruncated: boolean, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { code: number, length: string, roundtripMs: number, createdAt: string } | null } }> } };

export type HistoryRequestQueryVariables = Exact<{
  id: string | number;
}>;


export type HistoryRequestQuery = { request: { id: string, head: string, bodyLength: string, body: string, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { head: string, bodyLength: string, body: string } | null } | null };

export type HistoryRequestAddedSubscriptionVariables = Exact<{
  query?: string | null | undefined;
  scopeId?: string | number | null | undefined;
}>;


export type HistoryRequestAddedSubscription = { requestAdded: { id: string, source: RequestSource, host: string, port: number, tls: boolean, method: string, path: string, query: string, ext: string, length: string, createdAt: string, color: RequestColor | null, note: string | null, isDropped: boolean, isTruncated: boolean, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { code: number, length: string, roundtripMs: number, createdAt: string } | null } };

export type HistoryRequestUpdatedSubscriptionVariables = Exact<{
  query?: string | null | undefined;
  scopeId?: string | number | null | undefined;
}>;


export type HistoryRequestUpdatedSubscription = { requestUpdated: { matches: boolean, request: { id: string, source: RequestSource, host: string, port: number, tls: boolean, method: string, path: string, query: string, ext: string, length: string, createdAt: string, color: RequestColor | null, note: string | null, isDropped: boolean, isTruncated: boolean, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { code: number, length: string, roundtripMs: number, createdAt: string } | null } } };

export type HistoryRequestsDeletedSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type HistoryRequestsDeletedSubscription = { requestsDeleted: { ids: Array<string> } };

export type HistoryCompletionsQueryVariables = Exact<{
  text: string;
  cursor: number;
}>;


export type HistoryCompletionsQuery = { queryCompletions: { from: number, to: number, items: Array<{ label: string, insertText: string, kind: CompletionKind, detail: string, isExtension: boolean }> } };

export type HistorySetColorMutationVariables = Exact<{
  ids: Array<string | number> | string | number;
  color?: RequestColor | null | undefined;
}>;


export type HistorySetColorMutation = { setRequestColor: Array<{ id: string, color: RequestColor | null }> };

export type HistorySetNoteMutationVariables = Exact<{
  id: string | number;
  note?: string | null | undefined;
}>;


export type HistorySetNoteMutation = { setRequestNote: { id: string, note: string | null } };

export type HistoryDeleteRequestsMutationVariables = Exact<{
  ids: Array<string | number> | string | number;
}>;


export type HistoryDeleteRequestsMutation = { deleteRequests: string };

export type MatchReplaceRuleFieldsFragment = { id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } };

export type MatchReplaceCollectionFieldsFragment = { id: string, name: string, rules: Array<{ id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } }> };

export type MatchReplaceCollectionsQueryVariables = Exact<{ [key: string]: never; }>;


export type MatchReplaceCollectionsQuery = { matchReplaceCollections: Array<{ id: string, name: string, rules: Array<{ id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } }> }> };

export type ExportMatchReplaceQueryVariables = Exact<{ [key: string]: never; }>;


export type ExportMatchReplaceQuery = { exportMatchReplace: string };

export type CreateMatchReplaceCollectionMutationVariables = Exact<{
  name: string;
}>;


export type CreateMatchReplaceCollectionMutation = { createMatchReplaceCollection: { id: string, name: string, rules: Array<{ id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } }> } };

export type RenameMatchReplaceCollectionMutationVariables = Exact<{
  id: string | number;
  name: string;
}>;


export type RenameMatchReplaceCollectionMutation = { renameMatchReplaceCollection: { id: string, name: string } };

export type DeleteMatchReplaceCollectionMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteMatchReplaceCollectionMutation = { deleteMatchReplaceCollection: string };

export type ReorderMatchReplaceCollectionsMutationVariables = Exact<{
  ids: Array<string | number> | string | number;
}>;


export type ReorderMatchReplaceCollectionsMutation = { reorderMatchReplaceCollections: Array<{ id: string, name: string, rules: Array<{ id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } }> }> };

export type CreateMatchReplaceRuleMutationVariables = Exact<{
  collectionId: string | number;
  input: MatchReplaceRuleInput;
}>;


export type CreateMatchReplaceRuleMutation = { createMatchReplaceRule: { id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } } };

export type UpdateMatchReplaceRuleMutationVariables = Exact<{
  id: string | number;
  input: MatchReplaceRuleInput;
}>;


export type UpdateMatchReplaceRuleMutation = { updateMatchReplaceRule: { id: string, name: string, enabled: boolean, condition: string, sources: Array<MatchReplaceSource>, section: MatchReplaceSection, operation: MatchReplaceOperation, term: string, matcher: { kind: MatchReplaceMatcherKind, value: string } } };

export type SetMatchReplaceRuleEnabledMutationVariables = Exact<{
  id: string | number;
  enabled: boolean;
}>;


export type SetMatchReplaceRuleEnabledMutation = { setMatchReplaceRuleEnabled: { id: string, enabled: boolean } };

export type MoveMatchReplaceRuleMutationVariables = Exact<{
  id: string | number;
  collectionId: string | number;
  index: number;
}>;


export type MoveMatchReplaceRuleMutation = { moveMatchReplaceRule: { id: string } };

export type DeleteMatchReplaceRuleMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteMatchReplaceRuleMutation = { deleteMatchReplaceRule: string };

export type ImportMatchReplaceMutationVariables = Exact<{
  json: string;
}>;


export type ImportMatchReplaceMutation = { importMatchReplace: number };

export type ReplaySessionRowFragment = { id: string, name: string, color: RequestColor | null, draft: string, connection: { host: string, port: number, tls: boolean } };

export type ReplayCollectionRowFragment = { id: string, name: string, isDefault: boolean, sessions: Array<{ id: string, name: string, color: RequestColor | null, draft: string, connection: { host: string, port: number, tls: boolean } }> };

export type ReplayCollectionsQueryVariables = Exact<{ [key: string]: never; }>;


export type ReplayCollectionsQuery = { replayCollections: Array<{ id: string, name: string, isDefault: boolean, sessions: Array<{ id: string, name: string, color: RequestColor | null, draft: string, connection: { host: string, port: number, tls: boolean } }> }> };

export type CreateReplayCollectionMutationVariables = Exact<{
  name: string;
}>;


export type CreateReplayCollectionMutation = { createReplayCollection: { id: string } };

export type RenameReplayCollectionMutationVariables = Exact<{
  id: string | number;
  name: string;
}>;


export type RenameReplayCollectionMutation = { renameReplayCollection: { id: string } };

export type DeleteReplayCollectionMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteReplayCollectionMutation = { deleteReplayCollection: string };

export type CreateReplaySessionMutationVariables = Exact<{
  input: CreateReplaySessionInput;
}>;


export type CreateReplaySessionMutation = { createReplaySession: { id: string } };

export type RenameReplaySessionMutationVariables = Exact<{
  id: string | number;
  name: string;
}>;


export type RenameReplaySessionMutation = { updateReplaySession: { id: string } };

export type SetReplaySessionColorMutationVariables = Exact<{
  id: string | number;
  color?: RequestColor | null | undefined;
}>;


export type SetReplaySessionColorMutation = { setReplaySessionColor: { id: string } };

export type DuplicateReplaySessionMutationVariables = Exact<{
  id: string | number;
}>;


export type DuplicateReplaySessionMutation = { duplicateReplaySession: { id: string } };

export type MoveReplaySessionMutationVariables = Exact<{
  id: string | number;
  collectionId: string | number;
  index: number;
}>;


export type MoveReplaySessionMutation = { moveReplaySession: { id: string } };

export type DeleteReplaySessionMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteReplaySessionMutation = { deleteReplaySession: string };

export type ReplayEntryMessageFragment = { id: string, head: string, bodyLength: string, body: string, response: { code: number, length: string, roundtripMs: number, head: string, bodyLength: string, body: string } | null };

export type ReplayEntryRowFragment = { id: string, error: string | null, createdAt: string, request: { id: string, head: string, bodyLength: string, body: string, response: { code: number, length: string, roundtripMs: number, head: string, bodyLength: string, body: string } | null } };

export type ReplaySessionDetailQueryVariables = Exact<{
  id: string | number;
}>;


export type ReplaySessionDetailQuery = { replaySession: { id: string, name: string, draft: string, sending: boolean, connection: { host: string, port: number, tls: boolean, sni: string | null }, options: { followRedirects: boolean, updateContentLength: boolean, timeoutMs: number, applyMatchReplace: boolean }, entries: { edges: Array<{ node: { id: string, error: string | null, createdAt: string, request: { id: string, head: string, bodyLength: string, body: string, response: { code: number, length: string, roundtripMs: number, head: string, bodyLength: string, body: string } | null } } }> } } | null };

export type UpdateReplaySessionDraftMutationVariables = Exact<{
  id: string | number;
  input: UpdateReplaySessionInput;
}>;


export type UpdateReplaySessionDraftMutation = { updateReplaySession: { id: string } };

export type SendReplaySessionMutationVariables = Exact<{
  id: string | number;
  input: SendReplaySessionInput;
}>;


export type SendReplaySessionMutation = { sendReplaySession: { id: string, error: string | null, createdAt: string, request: { id: string, head: string, bodyLength: string, body: string, response: { code: number, length: string, roundtripMs: number, head: string, bodyLength: string, body: string } | null } } };

export type CancelReplaySendMutationVariables = Exact<{
  id: string | number;
}>;


export type CancelReplaySendMutation = { cancelReplaySend: boolean };

export type ScopePresetFieldsFragment = { id: string, name: string, allow: Array<string>, deny: Array<string> };

export type ScopePresetsQueryVariables = Exact<{ [key: string]: never; }>;


export type ScopePresetsQuery = { scopePresets: Array<{ id: string, name: string, allow: Array<string>, deny: Array<string> }> };

export type ScopeMatchCountQueryVariables = Exact<{
  scopeId: string | number;
}>;


export type ScopeMatchCountQuery = { requests: { totalCount: number } };

export type CreateScopePresetMutationVariables = Exact<{
  name: string;
  allow: Array<string> | string;
  deny: Array<string> | string;
}>;


export type CreateScopePresetMutation = { createScopePreset: { id: string, name: string, allow: Array<string>, deny: Array<string> } };

export type UpdateScopePresetMutationVariables = Exact<{
  id: string | number;
  name: string;
  allow: Array<string> | string;
  deny: Array<string> | string;
}>;


export type UpdateScopePresetMutation = { updateScopePreset: { id: string, name: string, allow: Array<string>, deny: Array<string> } };

export type DuplicateScopePresetMutationVariables = Exact<{
  id: string | number;
}>;


export type DuplicateScopePresetMutation = { duplicateScopePreset: { id: string, name: string, allow: Array<string>, deny: Array<string> } };

export type DeleteScopePresetMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteScopePresetMutation = { deleteScopePreset: string };

export type ReorderScopePresetsMutationVariables = Exact<{
  ids: Array<string | number> | string | number;
}>;


export type ReorderScopePresetsMutation = { reorderScopePresets: Array<{ id: string, name: string, allow: Array<string>, deny: Array<string> }> };

export type SitemapNodesQueryVariables = Exact<{
  parentId?: string | number | null | undefined;
  query?: string | null | undefined;
  scopeId?: string | number | null | undefined;
  recursive: boolean;
}>;


export type SitemapNodesQuery = { sitemapNodes: Array<{ id: string, parentId: string | null, kind: SitemapNodeKind, label: string, url: string, host: string, requestCount: number, hasChildren: boolean, query: string }> };

export type SitemapRequestsQueryVariables = Exact<{
  query: string;
  scopeId?: string | number | null | undefined;
  after?: string | null | undefined;
  sort?: RequestSort | null | undefined;
}>;


export type SitemapRequestsQuery = { requests: { totalCount: number, pageInfo: { hasNextPage: boolean, endCursor: string | null }, edges: Array<{ node: { id: string, source: RequestSource, host: string, port: number, tls: boolean, method: string, path: string, query: string, ext: string, length: string, createdAt: string, color: RequestColor | null, note: string | null, isDropped: boolean, isTruncated: boolean, editedFrom: { id: string } | null, editedBy: { id: string } | null, response: { code: number, length: string, roundtripMs: number, createdAt: string } | null } }> } };

export type SitemapDeleteMutationVariables = Exact<{
  query: string;
  scopeId?: string | number | null | undefined;
}>;


export type SitemapDeleteMutation = { deleteRequests: string };

export type ProjectFieldsFragment = { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string };

export type ProjectsQueryVariables = Exact<{ [key: string]: never; }>;


export type ProjectsQuery = { projects: Array<{ id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string }> };

export type CreateProjectMutationVariables = Exact<{
  name: string;
  select?: boolean;
}>;


export type CreateProjectMutation = { createProject: { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string } };

export type RenameProjectMutationVariables = Exact<{
  id: string | number;
  name: string;
}>;


export type RenameProjectMutation = { renameProject: { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string } };

export type DuplicateProjectMutationVariables = Exact<{
  id: string | number;
  name?: string | null | undefined;
}>;


export type DuplicateProjectMutation = { duplicateProject: { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string } };

export type DeleteProjectMutationVariables = Exact<{
  id: string | number;
}>;


export type DeleteProjectMutation = { deleteProject: string };

export type SetDefaultProjectMutationVariables = Exact<{
  id?: string | number | null | undefined;
}>;


export type SetDefaultProjectMutation = { setDefaultProject: { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string } | null };

export type SelectProjectMutationVariables = Exact<{
  id: string | number;
}>;


export type SelectProjectMutation = { selectProject: { id: string, name: string, isTemporary: boolean, isDefault: boolean, isSelected: boolean, createdAt: string, lastOpenedAt: string | null, dbSizeBytes: string } };

export type ProjectChangedSubscriptionVariables = Exact<{ [key: string]: never; }>;


export type ProjectChangedSubscription = { projectChanged: { kind: ProjectChangeKind, projectId: string } };

export const EnvironmentFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<EnvironmentFieldsFragment, unknown>;
export const FilterPresetFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FilterPresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"FilterPreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"alias"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]} as unknown as DocumentNode<FilterPresetFieldsFragment, unknown>;
export const HistoryRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"query"}},{"kind":"Field","name":{"kind":"Name","value":"ext"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"isDropped"}},{"kind":"Field","name":{"kind":"Name","value":"isTruncated"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<HistoryRowFragment, unknown>;
export const HistoryMessageFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}}]} as unknown as DocumentNode<HistoryMessageFragment, unknown>;
export const MatchReplaceRuleFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}}]} as unknown as DocumentNode<MatchReplaceRuleFieldsFragment, unknown>;
export const MatchReplaceCollectionFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceCollectionFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rules"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}}]} as unknown as DocumentNode<MatchReplaceCollectionFieldsFragment, unknown>;
export const ReplaySessionRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplaySessionRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplaySession"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}}]}},{"kind":"Field","name":{"kind":"Name","value":"draft"}}]}}]} as unknown as DocumentNode<ReplaySessionRowFragment, unknown>;
export const ReplayCollectionRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayCollectionRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplayCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"sessions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplaySessionRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplaySessionRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplaySession"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}}]}},{"kind":"Field","name":{"kind":"Name","value":"draft"}}]}}]} as unknown as DocumentNode<ReplayCollectionRowFragment, unknown>;
export const ReplayEntryMessageFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}}]} as unknown as DocumentNode<ReplayEntryMessageFragment, unknown>;
export const ReplayEntryRowFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplayEntry"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"request"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayEntryMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}}]} as unknown as DocumentNode<ReplayEntryRowFragment, unknown>;
export const ScopePresetFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<ScopePresetFieldsFragment, unknown>;
export const ProjectFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<ProjectFieldsFragment, unknown>;
export const ServerInfoDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ServerInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"serverInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"commit"}},{"kind":"Field","name":{"kind":"Name","value":"goVersion"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"authRequired"}}]}}]}}]} as unknown as DocumentNode<ServerInfoQuery, ServerInfoQueryVariables>;
export const PingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"Ping"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ping"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"seq"}},{"kind":"Field","name":{"kind":"Name","value":"at"}}]}}]}}]} as unknown as DocumentNode<PingSubscription, PingSubscriptionVariables>;
export const EnvironmentsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Environments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"environments"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EnvironmentFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<EnvironmentsQuery, EnvironmentsQueryVariables>;
export const CreateEnvironmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateEnvironment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createEnvironment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EnvironmentFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<CreateEnvironmentMutation, CreateEnvironmentMutationVariables>;
export const RenameEnvironmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RenameEnvironment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"renameEnvironment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EnvironmentFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<RenameEnvironmentMutation, RenameEnvironmentMutationVariables>;
export const DeleteEnvironmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteEnvironment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteEnvironment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteEnvironmentMutation, DeleteEnvironmentMutationVariables>;
export const SetEnvironmentVariablesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetEnvironmentVariables"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"variables"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"EnvironmentVariableInput"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setEnvironmentVariables"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"variables"},"value":{"kind":"Variable","name":{"kind":"Name","value":"variables"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EnvironmentFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<SetEnvironmentVariablesMutation, SetEnvironmentVariablesMutationVariables>;
export const SelectEnvironmentDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SelectEnvironment"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"selectEnvironment"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"EnvironmentFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"EnvironmentFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Environment"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isGlobal"}},{"kind":"Field","name":{"kind":"Name","value":"selected"}},{"kind":"Field","name":{"kind":"Name","value":"variables"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"secret"}}]}}]}}]} as unknown as DocumentNode<SelectEnvironmentMutation, SelectEnvironmentMutationVariables>;
export const FilterPresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FilterPresets"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"filterPresets"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FilterPresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FilterPresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"FilterPreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"alias"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]} as unknown as DocumentNode<FilterPresetsQuery, FilterPresetsQueryVariables>;
export const FilterMatchCountDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"FilterMatchCount"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<FilterMatchCountQuery, FilterMatchCountQueryVariables>;
export const CreateFilterPresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateFilterPreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"alias"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createFilterPreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"alias"},"value":{"kind":"Variable","name":{"kind":"Name","value":"alias"}}},{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FilterPresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FilterPresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"FilterPreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"alias"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]} as unknown as DocumentNode<CreateFilterPresetMutation, CreateFilterPresetMutationVariables>;
export const UpdateFilterPresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateFilterPreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"alias"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateFilterPreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"alias"},"value":{"kind":"Variable","name":{"kind":"Name","value":"alias"}}},{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FilterPresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FilterPresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"FilterPreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"alias"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]} as unknown as DocumentNode<UpdateFilterPresetMutation, UpdateFilterPresetMutationVariables>;
export const DeleteFilterPresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteFilterPreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteFilterPreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteFilterPresetMutation, DeleteFilterPresetMutationVariables>;
export const ReorderFilterPresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReorderFilterPresets"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"ids"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reorderFilterPresets"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"ids"},"value":{"kind":"Variable","name":{"kind":"Name","value":"ids"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FilterPresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FilterPresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"FilterPreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"alias"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]} as unknown as DocumentNode<ReorderFilterPresetsMutation, ReorderFilterPresetsMutationVariables>;
export const ExportFilterPresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExportFilterPresets"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"exportFilterPresets"}}]}}]} as unknown as DocumentNode<ExportFilterPresetsQuery, ExportFilterPresetsQueryVariables>;
export const ImportFilterPresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ImportFilterPresets"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"json"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"importFilterPresets"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"json"},"value":{"kind":"Variable","name":{"kind":"Name","value":"json"}}}]}]}}]} as unknown as DocumentNode<ImportFilterPresetsMutation, ImportFilterPresetsMutationVariables>;
export const HistoryRequestsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HistoryRequests"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sort"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestSort"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}},{"kind":"Argument","name":{"kind":"Name","value":"sort"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sort"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}},{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HistoryRow"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"query"}},{"kind":"Field","name":{"kind":"Name","value":"ext"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"isDropped"}},{"kind":"Field","name":{"kind":"Name","value":"isTruncated"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<HistoryRequestsQuery, HistoryRequestsQueryVariables>;
export const HistoryRequestDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HistoryRequest"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"request"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HistoryMessage"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}}]} as unknown as DocumentNode<HistoryRequestQuery, HistoryRequestQueryVariables>;
export const HistoryRequestAddedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"HistoryRequestAdded"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestAdded"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HistoryRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"query"}},{"kind":"Field","name":{"kind":"Name","value":"ext"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"isDropped"}},{"kind":"Field","name":{"kind":"Name","value":"isTruncated"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<HistoryRequestAddedSubscription, HistoryRequestAddedSubscriptionVariables>;
export const HistoryRequestUpdatedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"HistoryRequestUpdated"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestUpdated"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"matches"}},{"kind":"Field","name":{"kind":"Name","value":"request"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HistoryRow"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"query"}},{"kind":"Field","name":{"kind":"Name","value":"ext"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"isDropped"}},{"kind":"Field","name":{"kind":"Name","value":"isTruncated"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<HistoryRequestUpdatedSubscription, HistoryRequestUpdatedSubscriptionVariables>;
export const HistoryRequestsDeletedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"HistoryRequestsDeleted"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestsDeleted"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ids"}}]}}]}}]} as unknown as DocumentNode<HistoryRequestsDeletedSubscription, HistoryRequestsDeletedSubscriptionVariables>;
export const HistoryCompletionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"HistoryCompletions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"text"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"cursor"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"queryCompletions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"language"},"value":{"kind":"EnumValue","value":"HTTPQL"}},{"kind":"Argument","name":{"kind":"Name","value":"text"},"value":{"kind":"Variable","name":{"kind":"Name","value":"text"}}},{"kind":"Argument","name":{"kind":"Name","value":"cursor"},"value":{"kind":"Variable","name":{"kind":"Name","value":"cursor"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"from"}},{"kind":"Field","name":{"kind":"Name","value":"to"}},{"kind":"Field","name":{"kind":"Name","value":"items"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"insertText"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"detail"}},{"kind":"Field","name":{"kind":"Name","value":"isExtension"}}]}}]}}]}}]} as unknown as DocumentNode<HistoryCompletionsQuery, HistoryCompletionsQueryVariables>;
export const HistorySetColorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"HistorySetColor"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"ids"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"color"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestColor"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRequestColor"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"ids"},"value":{"kind":"Variable","name":{"kind":"Name","value":"ids"}}},{"kind":"Argument","name":{"kind":"Name","value":"color"},"value":{"kind":"Variable","name":{"kind":"Name","value":"color"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"color"}}]}}]}}]} as unknown as DocumentNode<HistorySetColorMutation, HistorySetColorMutationVariables>;
export const HistorySetNoteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"HistorySetNote"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"note"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setRequestNote"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"note"},"value":{"kind":"Variable","name":{"kind":"Name","value":"note"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"note"}}]}}]}}]} as unknown as DocumentNode<HistorySetNoteMutation, HistorySetNoteMutationVariables>;
export const HistoryDeleteRequestsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"HistoryDeleteRequests"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"ids"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRequests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"ids"},"value":{"kind":"Variable","name":{"kind":"Name","value":"ids"}}}]}]}}]} as unknown as DocumentNode<HistoryDeleteRequestsMutation, HistoryDeleteRequestsMutationVariables>;
export const MatchReplaceCollectionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"MatchReplaceCollections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"matchReplaceCollections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceCollectionFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceCollectionFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rules"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}}]} as unknown as DocumentNode<MatchReplaceCollectionsQuery, MatchReplaceCollectionsQueryVariables>;
export const ExportMatchReplaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExportMatchReplace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"exportMatchReplace"}}]}}]} as unknown as DocumentNode<ExportMatchReplaceQuery, ExportMatchReplaceQueryVariables>;
export const CreateMatchReplaceCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateMatchReplaceCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createMatchReplaceCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceCollectionFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceCollectionFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rules"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}}]} as unknown as DocumentNode<CreateMatchReplaceCollectionMutation, CreateMatchReplaceCollectionMutationVariables>;
export const RenameMatchReplaceCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RenameMatchReplaceCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"renameMatchReplaceCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}}]}}]} as unknown as DocumentNode<RenameMatchReplaceCollectionMutation, RenameMatchReplaceCollectionMutationVariables>;
export const DeleteMatchReplaceCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMatchReplaceCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMatchReplaceCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteMatchReplaceCollectionMutation, DeleteMatchReplaceCollectionMutationVariables>;
export const ReorderMatchReplaceCollectionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReorderMatchReplaceCollections"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"ids"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reorderMatchReplaceCollections"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"ids"},"value":{"kind":"Variable","name":{"kind":"Name","value":"ids"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceCollectionFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceCollectionFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"rules"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}}]} as unknown as DocumentNode<ReorderMatchReplaceCollectionsMutation, ReorderMatchReplaceCollectionsMutationVariables>;
export const CreateMatchReplaceRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateMatchReplaceRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRuleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createMatchReplaceRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"collectionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}}]} as unknown as DocumentNode<CreateMatchReplaceRuleMutation, CreateMatchReplaceRuleMutationVariables>;
export const UpdateMatchReplaceRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMatchReplaceRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRuleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMatchReplaceRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MatchReplaceRuleFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MatchReplaceRuleFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MatchReplaceRule"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"sources"}},{"kind":"Field","name":{"kind":"Name","value":"section"}},{"kind":"Field","name":{"kind":"Name","value":"operation"}},{"kind":"Field","name":{"kind":"Name","value":"matcher"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"value"}}]}},{"kind":"Field","name":{"kind":"Name","value":"term"}}]}}]} as unknown as DocumentNode<UpdateMatchReplaceRuleMutation, UpdateMatchReplaceRuleMutationVariables>;
export const SetMatchReplaceRuleEnabledDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetMatchReplaceRuleEnabled"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"enabled"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setMatchReplaceRuleEnabled"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"enabled"},"value":{"kind":"Variable","name":{"kind":"Name","value":"enabled"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}}]}}]}}]} as unknown as DocumentNode<SetMatchReplaceRuleEnabledMutation, SetMatchReplaceRuleEnabledMutationVariables>;
export const MoveMatchReplaceRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MoveMatchReplaceRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"index"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"moveMatchReplaceRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"collectionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}}},{"kind":"Argument","name":{"kind":"Name","value":"index"},"value":{"kind":"Variable","name":{"kind":"Name","value":"index"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<MoveMatchReplaceRuleMutation, MoveMatchReplaceRuleMutationVariables>;
export const DeleteMatchReplaceRuleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMatchReplaceRule"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMatchReplaceRule"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteMatchReplaceRuleMutation, DeleteMatchReplaceRuleMutationVariables>;
export const ImportMatchReplaceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ImportMatchReplace"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"json"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"importMatchReplace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"json"},"value":{"kind":"Variable","name":{"kind":"Name","value":"json"}}}]}]}}]} as unknown as DocumentNode<ImportMatchReplaceMutation, ImportMatchReplaceMutationVariables>;
export const ReplayCollectionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ReplayCollections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"replayCollections"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayCollectionRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplaySessionRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplaySession"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}}]}},{"kind":"Field","name":{"kind":"Name","value":"draft"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayCollectionRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplayCollection"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"sessions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplaySessionRow"}}]}}]}}]} as unknown as DocumentNode<ReplayCollectionsQuery, ReplayCollectionsQueryVariables>;
export const CreateReplayCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateReplayCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createReplayCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateReplayCollectionMutation, CreateReplayCollectionMutationVariables>;
export const RenameReplayCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RenameReplayCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"renameReplayCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<RenameReplayCollectionMutation, RenameReplayCollectionMutationVariables>;
export const DeleteReplayCollectionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteReplayCollection"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteReplayCollection"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteReplayCollectionMutation, DeleteReplayCollectionMutationVariables>;
export const CreateReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateReplaySessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateReplaySessionMutation, CreateReplaySessionMutationVariables>;
export const RenameReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RenameReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"ObjectValue","fields":[{"kind":"ObjectField","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}]}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<RenameReplaySessionMutation, RenameReplaySessionMutationVariables>;
export const SetReplaySessionColorDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetReplaySessionColor"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"color"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestColor"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setReplaySessionColor"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"color"},"value":{"kind":"Variable","name":{"kind":"Name","value":"color"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<SetReplaySessionColorMutation, SetReplaySessionColorMutationVariables>;
export const DuplicateReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DuplicateReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"duplicateReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<DuplicateReplaySessionMutation, DuplicateReplaySessionMutationVariables>;
export const MoveReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"MoveReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"index"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"moveReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"collectionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"collectionId"}}},{"kind":"Argument","name":{"kind":"Name","value":"index"},"value":{"kind":"Variable","name":{"kind":"Name","value":"index"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<MoveReplaySessionMutation, MoveReplaySessionMutationVariables>;
export const DeleteReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteReplaySessionMutation, DeleteReplaySessionMutationVariables>;
export const ReplaySessionDetailDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ReplaySessionDetail"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"replaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"connection"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"sni"}}]}},{"kind":"Field","name":{"kind":"Name","value":"options"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"followRedirects"}},{"kind":"Field","name":{"kind":"Name","value":"updateContentLength"}},{"kind":"Field","name":{"kind":"Name","value":"timeoutMs"}},{"kind":"Field","name":{"kind":"Name","value":"applyMatchReplace"}}]}},{"kind":"Field","name":{"kind":"Name","value":"draft"}},{"kind":"Field","name":{"kind":"Name","value":"sending"}},{"kind":"Field","name":{"kind":"Name","value":"entries"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"100"}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayEntryRow"}}]}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplayEntry"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"request"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayEntryMessage"}}]}}]}}]} as unknown as DocumentNode<ReplaySessionDetailQuery, ReplaySessionDetailQueryVariables>;
export const UpdateReplaySessionDraftDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateReplaySessionDraft"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateReplaySessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<UpdateReplaySessionDraftMutation, UpdateReplaySessionDraftMutationVariables>;
export const SendReplaySessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SendReplaySession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SendReplaySessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sendReplaySession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayEntryRow"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryMessage"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"head"}},{"kind":"Field","name":{"kind":"Name","value":"bodyLength"}},{"kind":"Field","name":{"kind":"Name","value":"body"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"IntValue","value":"4194304"}},{"kind":"Argument","name":{"kind":"Name","value":"decode"},"value":{"kind":"BooleanValue","value":true}}]}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ReplayEntryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ReplayEntry"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"error"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"request"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ReplayEntryMessage"}}]}}]}}]} as unknown as DocumentNode<SendReplaySessionMutation, SendReplaySessionMutationVariables>;
export const CancelReplaySendDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelReplaySend"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelReplaySend"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<CancelReplaySendMutation, CancelReplaySendMutationVariables>;
export const ScopePresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ScopePresets"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"scopePresets"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ScopePresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<ScopePresetsQuery, ScopePresetsQueryVariables>;
export const ScopeMatchCountDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ScopeMatchCount"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"1"}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<ScopeMatchCountQuery, ScopeMatchCountQueryVariables>;
export const CreateScopePresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateScopePreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"allow"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"deny"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createScopePreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"allow"},"value":{"kind":"Variable","name":{"kind":"Name","value":"allow"}}},{"kind":"Argument","name":{"kind":"Name","value":"deny"},"value":{"kind":"Variable","name":{"kind":"Name","value":"deny"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ScopePresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<CreateScopePresetMutation, CreateScopePresetMutationVariables>;
export const UpdateScopePresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateScopePreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"allow"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"deny"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateScopePreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"allow"},"value":{"kind":"Variable","name":{"kind":"Name","value":"allow"}}},{"kind":"Argument","name":{"kind":"Name","value":"deny"},"value":{"kind":"Variable","name":{"kind":"Name","value":"deny"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ScopePresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<UpdateScopePresetMutation, UpdateScopePresetMutationVariables>;
export const DuplicateScopePresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DuplicateScopePreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"duplicateScopePreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ScopePresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<DuplicateScopePresetMutation, DuplicateScopePresetMutationVariables>;
export const DeleteScopePresetDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteScopePreset"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteScopePreset"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteScopePresetMutation, DeleteScopePresetMutationVariables>;
export const ReorderScopePresetsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ReorderScopePresets"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"ids"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"reorderScopePresets"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"ids"},"value":{"kind":"Variable","name":{"kind":"Name","value":"ids"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ScopePresetFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ScopePresetFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ScopePreset"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"allow"}},{"kind":"Field","name":{"kind":"Name","value":"deny"}}]}}]} as unknown as DocumentNode<ReorderScopePresetsMutation, ReorderScopePresetsMutationVariables>;
export const SitemapNodesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SitemapNodes"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"parentId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"recursive"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sitemapNodes"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"parentId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"parentId"}}},{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}},{"kind":"Argument","name":{"kind":"Name","value":"recursive"},"value":{"kind":"Variable","name":{"kind":"Name","value":"recursive"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"parentId"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"label"}},{"kind":"Field","name":{"kind":"Name","value":"url"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"requestCount"}},{"kind":"Field","name":{"kind":"Name","value":"hasChildren"}},{"kind":"Field","name":{"kind":"Name","value":"query"}}]}}]}}]} as unknown as DocumentNode<SitemapNodesQuery, SitemapNodesQueryVariables>;
export const SitemapRequestsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SitemapRequests"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sort"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestSort"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"IntValue","value":"1000"}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}},{"kind":"Argument","name":{"kind":"Name","value":"sort"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sort"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}},{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HistoryRow"}}]}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HistoryRow"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HttpRequest"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"host"}},{"kind":"Field","name":{"kind":"Name","value":"port"}},{"kind":"Field","name":{"kind":"Name","value":"tls"}},{"kind":"Field","name":{"kind":"Name","value":"method"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"query"}},{"kind":"Field","name":{"kind":"Name","value":"ext"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"color"}},{"kind":"Field","name":{"kind":"Name","value":"note"}},{"kind":"Field","name":{"kind":"Name","value":"isDropped"}},{"kind":"Field","name":{"kind":"Name","value":"isTruncated"}},{"kind":"Field","name":{"kind":"Name","value":"editedFrom"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"editedBy"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}},{"kind":"Field","name":{"kind":"Name","value":"response"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"length"}},{"kind":"Field","name":{"kind":"Name","value":"roundtripMs"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<SitemapRequestsQuery, SitemapRequestsQueryVariables>;
export const SitemapDeleteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SitemapDelete"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"query"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteRequests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"query"},"value":{"kind":"Variable","name":{"kind":"Name","value":"query"}}},{"kind":"Argument","name":{"kind":"Name","value":"scopeId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"scopeId"}}}]}]}}]} as unknown as DocumentNode<SitemapDeleteMutation, SitemapDeleteMutationVariables>;
export const ProjectsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Projects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projects"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<ProjectsQuery, ProjectsQueryVariables>;
export const CreateProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"select"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}}},"defaultValue":{"kind":"BooleanValue","value":false}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"select"},"value":{"kind":"Variable","name":{"kind":"Name","value":"select"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<CreateProjectMutation, CreateProjectMutationVariables>;
export const RenameProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RenameProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"renameProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<RenameProjectMutation, RenameProjectMutationVariables>;
export const DuplicateProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DuplicateProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"duplicateProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<DuplicateProjectMutation, DuplicateProjectMutationVariables>;
export const DeleteProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteProjectMutation, DeleteProjectMutationVariables>;
export const SetDefaultProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SetDefaultProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"setDefaultProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<SetDefaultProjectMutation, SetDefaultProjectMutationVariables>;
export const SelectProjectDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SelectProject"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"selectProject"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProjectFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProjectFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"Project"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"isTemporary"}},{"kind":"Field","name":{"kind":"Name","value":"isDefault"}},{"kind":"Field","name":{"kind":"Name","value":"isSelected"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"lastOpenedAt"}},{"kind":"Field","name":{"kind":"Name","value":"dbSizeBytes"}}]}}]} as unknown as DocumentNode<SelectProjectMutation, SelectProjectMutationVariables>;
export const ProjectChangedDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"ProjectChanged"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"projectChanged"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"projectId"}}]}}]}}]} as unknown as DocumentNode<ProjectChangedSubscription, ProjectChangedSubscriptionVariables>;