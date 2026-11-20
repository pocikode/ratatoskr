// Section / operation / matcher dependencies of a Match & Replace rule. Mirrors the table in
// internal/api/graphql/schema/matchreplace.graphqls (T-099); the server stays authoritative.
import type {
  MatchReplaceMatcherKind,
  MatchReplaceOperation,
  MatchReplaceSection,
  MatchReplaceSource,
} from '$lib/api/generated/graphql';

export const SOURCES: { value: MatchReplaceSource; label: string }[] = [
  { value: 'PROXY', label: 'Proxy' },
  { value: 'INTERCEPT', label: 'Intercept' },
  { value: 'REPLAY', label: 'Replay' },
  { value: 'AUTOMATE', label: 'Automate' },
  { value: 'WORKFLOW', label: 'Workflow' },
  { value: 'PLUGIN', label: 'Plugin' },
];

const RAW_ONLY: MatchReplaceOperation[] = ['UPDATE_RAW'];
const ITEMS: MatchReplaceOperation[] = ['UPDATE_RAW', 'UPDATE_VALUE', 'ADD', 'REMOVE'];

export const SECTIONS: {
  value: MatchReplaceSection;
  label: string;
  operations: MatchReplaceOperation[];
}[] = [
  { value: 'REQUEST_FIRST_LINE', label: 'Request first line', operations: RAW_ONLY },
  { value: 'REQUEST_METHOD', label: 'Request method', operations: ['UPDATE'] },
  { value: 'REQUEST_PATH', label: 'Request path', operations: RAW_ONLY },
  { value: 'REQUEST_QUERY', label: 'Request query', operations: ITEMS },
  { value: 'REQUEST_HEADER', label: 'Request header', operations: ITEMS },
  { value: 'REQUEST_BODY', label: 'Request body', operations: RAW_ONLY },
  { value: 'RESPONSE_FIRST_LINE', label: 'Response first line', operations: RAW_ONLY },
  { value: 'RESPONSE_STATUS_CODE', label: 'Response status code', operations: ['UPDATE'] },
  { value: 'RESPONSE_HEADER', label: 'Response header', operations: ITEMS },
  { value: 'RESPONSE_BODY', label: 'Response body', operations: RAW_ONLY },
];

export const OPERATION_LABELS: Record<MatchReplaceOperation, string> = {
  UPDATE_RAW: 'Update raw',
  UPDATE: 'Update',
  UPDATE_VALUE: 'Update value',
  ADD: 'Add',
  REMOVE: 'Remove',
};

export const MATCHER_LABELS: Record<MatchReplaceMatcherKind, string> = {
  FULL: 'Full',
  STRING: 'String',
  REGEX: 'Regex',
  NAME: 'Name',
};

export function operationsFor(section: MatchReplaceSection): MatchReplaceOperation[] {
  return SECTIONS.find((s) => s.value === section)?.operations ?? [];
}

/** `ADD` ignores the matcher, so it has none to pick. */
export function matchersFor(operation: MatchReplaceOperation): MatchReplaceMatcherKind[] {
  if (operation === 'ADD') return [];
  return operation === 'UPDATE_VALUE' || operation === 'REMOVE'
    ? ['FULL', 'STRING', 'REGEX', 'NAME']
    : ['FULL', 'STRING', 'REGEX'];
}
