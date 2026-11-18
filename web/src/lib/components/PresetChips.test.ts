import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import PresetChips from './PresetChips.svelte';

const presets = [
  { id: '1', name: 'No assets', alias: 'no-assets' },
  { id: '2', name: 'Errors', alias: null },
];

describe('PresetChips', () => {
  it('applies a preset on click and marks the active one', async () => {
    const onapply = vi.fn();
    render(PresetChips, { presets, active: 'Errors', onapply });

    expect(screen.getByRole('group', { name: 'Filter presets' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Errors' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'No assets' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );

    await userEvent.click(screen.getByRole('button', { name: 'No assets' }));
    expect(onapply).toHaveBeenCalledWith(presets[0]);
  });

  it('renders nothing without presets', () => {
    render(PresetChips, { presets: [], onapply: () => {} });

    expect(screen.queryByRole('group', { name: 'Filter presets' })).not.toBeInTheDocument();
  });
});
