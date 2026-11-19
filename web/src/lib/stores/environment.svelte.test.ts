import { describe, expect, it, vi } from 'vitest';
import { createMockClient } from '$lib/api/mocks/exchange';
import { projects } from './project.svelte';
import { EnvironmentStore } from './environment.svelte';

const store = () => {
  const client = createMockClient();
  return new EnvironmentStore(() => client);
};

describe('EnvironmentStore', () => {
  it('resolves variables from the selected environment over Global', async () => {
    const env = store();
    await env.refresh();

    // Mock: Global has `userAgent`; Staging is selected and has `host` and `token`.
    expect(env.selected?.name).toBe('Staging');
    expect(env.variables.get('host')).toBe('staging.example.test');
    expect(env.variables.get('userAgent')).toBe('Mozilla/5.0 (mock)');

    await env.select('env2');
    expect(env.selected?.name).toBe('Production');
    expect(env.variables.get('host')).toBe('www.example.com');

    await env.select(null);
    expect(env.selected).toBeNull();
    expect(env.variables.has('host')).toBe(false); // Global has no `host`
    expect(env.variables.get('userAgent')).toBe('Mozilla/5.0 (mock)');
  });

  it('refetches after its own mutations', async () => {
    const env = store();
    await env.refresh();
    const staging = env.selected!;

    expect(
      await env.saveVariables(staging.id, [{ name: 'host', value: 'qa.test', secret: false }]),
    ).toBe(true);
    expect(env.selected?.variables).toEqual([{ name: 'host', value: 'qa.test', secret: false }]);
    expect(env.variables.get('host')).toBe('qa.test');

    expect(await env.create('QA')).toBe(true);
    const qa = env.environments.find((e) => e.name === 'QA')!;
    expect(await env.rename(qa.id, 'QA 2')).toBe(true);
    expect(env.environments.map((e) => e.name)).toContain('QA 2');
    expect(await env.remove(qa.id)).toBe(true);
    expect(env.environments.map((e) => e.name)).not.toContain('QA 2');
  });

  it('keeps the data and records the message when a mutation fails', async () => {
    const env = store();
    await env.refresh();

    const saved = await env.saveVariables(env.selected!.id, [
      { name: 'host', value: 'a', secret: false },
      { name: 'host', value: 'b', secret: false },
    ]);

    expect(saved).toBe(false);
    expect(env.error).toMatch(/duplicate variable name "host"/);
    expect(env.selected?.variables).toHaveLength(2);
  });

  it('loads on start and refetches on a project switch', async () => {
    const env = store();
    const onReset = vi.spyOn(projects, 'onReset');
    const stop = env.start();
    await vi.waitFor(() => expect(env.environments).toHaveLength(3));

    env.environments = [];
    onReset.mock.calls[0]![0](); // what the project store runs when another project is selected
    await vi.waitFor(() => expect(env.environments).toHaveLength(3));

    stop();
    onReset.mockRestore();
  });
});
