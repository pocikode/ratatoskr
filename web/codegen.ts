import type { CodegenConfig } from '@graphql-codegen/cli';

// Typed documents for every operation in src/**/*.graphql (docs/06-frontend-conventions.md §4).
// The schema is read from the Go side's schema files, so `make gen` needs no running server.
const config: CodegenConfig = {
  schema: '../internal/api/graphql/schema/*.graphqls',
  documents: ['src/**/*.graphql'],
  generates: {
    'src/lib/api/generated/': {
      preset: 'client',
      presetConfig: { fragmentMasking: false },
      config: {
        // Wire formats of the custom scalars: docs/03-architecture.md §6.
        scalars: {
          Time: 'string',
          Bytes: 'string',
          Int64: 'string',
        },
        strictScalars: true,
        enumsAsTypes: true,
        useTypeImports: true,
      },
    },
  },
};

export default config;
