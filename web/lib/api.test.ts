import { describe, it, expect, afterEach, vi } from 'vitest';
import { getHealth, getProfiles, getUsage, apiBaseUrl } from './api';

describe('api client', () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  it('defaults to same-origin API paths', async () => {
    vi.stubEnv('NEXT_PUBLIC_API_BASE', '');
    const spy = vi.fn(async () =>
      new Response(JSON.stringify({ ok: true, version: '0.1.0-dev' }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    );
    global.fetch = spy as unknown as typeof fetch;

    expect(apiBaseUrl()).toBe('');

    await getHealth();

    expect(spy).toHaveBeenCalledWith('/api/health', {
      headers: { Accept: 'application/json' },
      cache: 'no-store',
    });
  });

  it('uses NEXT_PUBLIC_API_BASE when configured', () => {
    vi.stubEnv('NEXT_PUBLIC_API_BASE', 'http://127.0.0.1:7777/');

    expect(apiBaseUrl()).toBe('http://127.0.0.1:7777');
  });

  it('getHealth parses { ok, version } from /api/health', async () => {
    global.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ ok: true, version: '0.1.0-dev' }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as unknown as typeof fetch;

    const out = await getHealth();
    expect(out.ok).toBe(true);
    expect(out.version).toBe('0.1.0-dev');
  });

  it('getProfiles parses an array of ProfileWithTotals', async () => {
    global.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify([
          {
            name: 'work',
            config_dir: '/x',
            color: '#3B82F6',
            created_at: '2026-05-19T12:00:00Z',
            last_used_at: '2026-05-19T12:00:00Z',
            today: {
              usage: {
                input_tokens: 1,
                output_tokens: 2,
                cache_read_tokens: 3,
                cache_create_tokens: 4,
              },
              estimated_usd: 0.42,
            },
          },
        ]),
        { headers: { 'Content-Type': 'application/json' } },
      ),
    ) as unknown as typeof fetch;

    const out = await getProfiles();
    expect(out).toHaveLength(1);
    expect(out[0]?.name).toBe('work');
    expect(out[0]?.today.estimated_usd).toBe(0.42);
  });

  it('getUsage forwards query params and parses rows', async () => {
    const spy = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input.toString();
      expect(url).toContain('/api/usage');
      expect(url).toContain('profile=work');
      expect(url).toContain('since=7d');
      return new Response(
        JSON.stringify({
          rows: [],
          total: {
            usage: {
              input_tokens: 0,
              output_tokens: 0,
              cache_read_tokens: 0,
              cache_create_tokens: 0,
            },
            estimated_usd: 0,
          },
        }),
        { headers: { 'Content-Type': 'application/json' } },
      );
    });
    global.fetch = spy as unknown as typeof fetch;

    const out = await getUsage({ profile: 'work', since: '7d' });
    expect(out.rows).toEqual([]);
    expect(out.total.estimated_usd).toBe(0);
    expect(spy).toHaveBeenCalledOnce();
  });

  it('throws on non-2xx with a useful message', async () => {
    global.fetch = vi.fn(async () =>
      new Response('boom', { status: 500 }),
    ) as unknown as typeof fetch;

    await expect(getHealth()).rejects.toThrow(/500/);
  });
});
