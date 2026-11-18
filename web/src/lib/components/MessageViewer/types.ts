export const TEXT_MODES = ['raw', 'pretty', 'hex', 'table'] as const;

export type ViewerMode = (typeof TEXT_MODES)[number] | 'preview';

export const MODE_LABELS: Record<ViewerMode, string> = {
  raw: 'Raw',
  pretty: 'Pretty',
  hex: 'Hex',
  table: 'Table',
  preview: 'Preview',
};
