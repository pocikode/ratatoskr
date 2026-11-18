import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import ScopeSelector from './ScopeSelector.svelte';

const presets = [
  { id: '1', name: 'Example app' },
  { id: '2', name: 'API only' },
];

describe('ScopeSelector', () => {
  it('offers "No scope" plus every preset and reports the choice', async () => {
    const onchange = vi.fn();
    render(ScopeSelector, { presets, value: null, onchange });

    const select = screen.getByRole('combobox', { name: 'Scope' });
    expect(select).toHaveValue('');
    expect(screen.getAllByRole('option').map((option) => option.textContent)).toEqual([
      'No scope',
      'Example app',
      'API only',
    ]);

    await userEvent.selectOptions(select, '2');
    expect(onchange).toHaveBeenCalledWith('2');
  });

  it('falls back to "No scope" when the selected preset is gone', () => {
    render(ScopeSelector, { presets, value: 'deleted' });

    expect(screen.getByRole('combobox', { name: 'Scope' })).toHaveValue('');
  });
});
