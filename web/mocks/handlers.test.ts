import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { setupServer } from 'msw/node';
import { handlers } from './handlers';

const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const base = (process.env.NEXT_PUBLIC_API_BASE ?? 'http://127.0.0.1:7777').replace(
  /\/$/,
  '',
);

describe('MSW handlers cover every openapi.yaml endpoint', () => {
  it('GET /api/health', async () => {
    const res = await fetch(`${base}/api/health`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { ok: boolean; version: string };
    expect(body.ok).toBe(true);
    expect(typeof body.version).toBe('string');
  });

  it('GET /api/profiles', async () => {
    const res = await fetch(`${base}/api/profiles`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as unknown[];
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(3);
  });

  it('GET /api/usage with no filter', async () => {
    const res = await fetch(`${base}/api/usage`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { rows: unknown[]; total: unknown };
    expect(Array.isArray(body.rows)).toBe(true);
    expect(body.total).toBeDefined();
  });

  it('GET /api/usage filters by profile', async () => {
    const res = await fetch(`${base}/api/usage?profile=work`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { rows: { profile: string }[] };
    for (const r of body.rows) expect(r.profile).toBe('work');
  });

  it('GET /api/usage/live serves text/event-stream', async () => {
    const res = await fetch(`${base}/api/usage/live`);
    expect(res.status).toBe(200);
    expect(res.headers.get('content-type')).toContain('text/event-stream');
    const text = await res.text();
    expect(text).toContain('event: usage');
  });
});
