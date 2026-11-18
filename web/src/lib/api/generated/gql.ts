/* eslint-disable */
import * as types from './graphql';
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';

/**
 * Map of all GraphQL operations in the project.
 *
 * This map has several performance disadvantages:
 * 1. It is not tree-shakeable, so it will include all operations in the project.
 * 2. It is not minifiable, so the string of a GraphQL query will be multiple times inside the bundle.
 * 3. It does not support dead code elimination, so it will add unused operations.
 *
 * Therefore it is highly recommended to use the babel or swc plugin for production.
 * Learn more about it here: https://the-guild.dev/graphql/codegen/plugins/presets/preset-client#reducing-bundle-size
 */
type Documents = {
    "query ServerInfo {\n  serverInfo {\n    version\n    commit\n    goVersion\n    startedAt\n    authRequired\n  }\n}\n\nsubscription Ping {\n  ping {\n    seq\n    at\n  }\n}": typeof types.ServerInfoDocument,
    "fragment ProjectFields on Project {\n  id\n  name\n  isTemporary\n  isDefault\n  isSelected\n  createdAt\n  lastOpenedAt\n  dbSizeBytes\n}\n\nquery Projects {\n  projects {\n    ...ProjectFields\n  }\n}\n\nmutation CreateProject($name: String!, $select: Boolean! = false) {\n  createProject(name: $name, select: $select) {\n    ...ProjectFields\n  }\n}\n\nmutation RenameProject($id: ID!, $name: String!) {\n  renameProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DuplicateProject($id: ID!, $name: String) {\n  duplicateProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DeleteProject($id: ID!) {\n  deleteProject(id: $id)\n}\n\nmutation SetDefaultProject($id: ID) {\n  setDefaultProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nmutation SelectProject($id: ID!) {\n  selectProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nsubscription ProjectChanged {\n  projectChanged {\n    kind\n    projectId\n  }\n}": typeof types.ProjectFieldsFragmentDoc,
};
const documents: Documents = {
    "query ServerInfo {\n  serverInfo {\n    version\n    commit\n    goVersion\n    startedAt\n    authRequired\n  }\n}\n\nsubscription Ping {\n  ping {\n    seq\n    at\n  }\n}": types.ServerInfoDocument,
    "fragment ProjectFields on Project {\n  id\n  name\n  isTemporary\n  isDefault\n  isSelected\n  createdAt\n  lastOpenedAt\n  dbSizeBytes\n}\n\nquery Projects {\n  projects {\n    ...ProjectFields\n  }\n}\n\nmutation CreateProject($name: String!, $select: Boolean! = false) {\n  createProject(name: $name, select: $select) {\n    ...ProjectFields\n  }\n}\n\nmutation RenameProject($id: ID!, $name: String!) {\n  renameProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DuplicateProject($id: ID!, $name: String) {\n  duplicateProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DeleteProject($id: ID!) {\n  deleteProject(id: $id)\n}\n\nmutation SetDefaultProject($id: ID) {\n  setDefaultProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nmutation SelectProject($id: ID!) {\n  selectProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nsubscription ProjectChanged {\n  projectChanged {\n    kind\n    projectId\n  }\n}": types.ProjectFieldsFragmentDoc,
};

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 *
 *
 * @example
 * ```ts
 * const query = graphql(`query GetUser($id: ID!) { user(id: $id) { name } }`);
 * ```
 *
 * The query argument is unknown!
 * Please regenerate the types.
 */
export function graphql(source: string): unknown;

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "query ServerInfo {\n  serverInfo {\n    version\n    commit\n    goVersion\n    startedAt\n    authRequired\n  }\n}\n\nsubscription Ping {\n  ping {\n    seq\n    at\n  }\n}"): (typeof documents)["query ServerInfo {\n  serverInfo {\n    version\n    commit\n    goVersion\n    startedAt\n    authRequired\n  }\n}\n\nsubscription Ping {\n  ping {\n    seq\n    at\n  }\n}"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "fragment ProjectFields on Project {\n  id\n  name\n  isTemporary\n  isDefault\n  isSelected\n  createdAt\n  lastOpenedAt\n  dbSizeBytes\n}\n\nquery Projects {\n  projects {\n    ...ProjectFields\n  }\n}\n\nmutation CreateProject($name: String!, $select: Boolean! = false) {\n  createProject(name: $name, select: $select) {\n    ...ProjectFields\n  }\n}\n\nmutation RenameProject($id: ID!, $name: String!) {\n  renameProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DuplicateProject($id: ID!, $name: String) {\n  duplicateProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DeleteProject($id: ID!) {\n  deleteProject(id: $id)\n}\n\nmutation SetDefaultProject($id: ID) {\n  setDefaultProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nmutation SelectProject($id: ID!) {\n  selectProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nsubscription ProjectChanged {\n  projectChanged {\n    kind\n    projectId\n  }\n}"): (typeof documents)["fragment ProjectFields on Project {\n  id\n  name\n  isTemporary\n  isDefault\n  isSelected\n  createdAt\n  lastOpenedAt\n  dbSizeBytes\n}\n\nquery Projects {\n  projects {\n    ...ProjectFields\n  }\n}\n\nmutation CreateProject($name: String!, $select: Boolean! = false) {\n  createProject(name: $name, select: $select) {\n    ...ProjectFields\n  }\n}\n\nmutation RenameProject($id: ID!, $name: String!) {\n  renameProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DuplicateProject($id: ID!, $name: String) {\n  duplicateProject(id: $id, name: $name) {\n    ...ProjectFields\n  }\n}\n\nmutation DeleteProject($id: ID!) {\n  deleteProject(id: $id)\n}\n\nmutation SetDefaultProject($id: ID) {\n  setDefaultProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nmutation SelectProject($id: ID!) {\n  selectProject(id: $id) {\n    ...ProjectFields\n  }\n}\n\nsubscription ProjectChanged {\n  projectChanged {\n    kind\n    projectId\n  }\n}"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;