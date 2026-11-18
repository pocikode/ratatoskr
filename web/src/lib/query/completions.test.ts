import { CompletionContext } from '@codemirror/autocomplete';
import { EditorState } from '@codemirror/state';
import { describe, expect, it, vi } from 'vitest';
import { completionSource, type QueryCompletion } from './completions';
import { docToByte } from './httpql';

const ITEM: QueryCompletion = {
  label: 'req',
  insertText: 'req',
  kind: 'NAMESPACE',
  detail: 'namespace',
  isExtension: false,
};

function context(doc: string, pos = doc.length) {
  return new CompletionContext(EditorState.create({ doc }), pos, false);
}

describe('completionSource', () => {
  it('asks the server with the cursor as a UTF-8 byte offset', async () => {
    const doc = '"é" req.ho';
    const fetch = vi.fn(async () => ({ from: 4, to: docToByte(doc, doc.length), items: [ITEM] }));
    const source = completionSource(fetch);

    const result = await source(context(doc));

    expect(fetch).toHaveBeenCalledWith(doc, docToByte(doc, doc.length));
    // Byte 4 is the space after `"é"`; the editor offset is 3.
    expect(result).toMatchObject({ from: 3, to: doc.length, filter: false });
  });

  it('maps byte spans back to editor positions around multi-byte text', async () => {
    const doc = '"é" ho';
    // Bytes: `"` 1, `é` 2, `"` 1, space 1, `ho` 2 → the token `ho` is bytes 5..7.
    const source = completionSource(async () => ({ from: 5, to: 7, items: [ITEM] }));

    expect(await source(context(doc))).toMatchObject({ from: 4, to: 6 });
  });

  it('tags the completion kinds and extension items', async () => {
    const source = completionSource(async () => ({
      from: 0,
      to: 0,
      items: [
        { ...ITEM, kind: 'FIELD', detail: 'string' },
        { ...ITEM, label: 'color', kind: 'FIELD', detail: '', isExtension: true },
        { ...ITEM, label: 'eq', kind: 'OPERATOR', detail: 'operator' },
      ],
    }));

    const result = await source(context(''));

    expect(result?.options.map((option) => option.type)).toEqual([
      'property',
      'property',
      'keyword',
    ]);
    expect(result?.options[1]).toMatchObject({ detail: 'extension (extension)' });
    expect(result?.options[2]).toMatchObject({ apply: 'req' });
  });

  it('returns null when the server has nothing to suggest', async () => {
    expect(await completionSource(async () => null)(context('req.'))).toBeNull();
    expect(
      await completionSource(async () => ({ from: 0, to: 0, items: [] }))(context('req.')),
    ).toBeNull();
  });
});
