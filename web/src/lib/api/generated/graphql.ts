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

/** A variable to store; same rules as `EnvironmentVariable`. */
export type EnvironmentVariableInput = {
  /** Variable name (see `EnvironmentVariable.name`). */
  name: string;
  /** Whether the value is a secret. Default false. */
  secret?: boolean;
  /** Value, at most 1 MiB. */
  value: string;
};

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