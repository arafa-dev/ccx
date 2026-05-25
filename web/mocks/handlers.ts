import { http, HttpResponse } from 'msw';
import {
  FIXTURE_DAEMON_STATUS,
  FIXTURE_HEADROOM,
  FIXTURE_HOOK_STATUS,
  FIXTURE_PROFILES,
  aggregateTotal,
  generateSessions,
  generateUsageRows,
} from './fixtures';

const configuredBase = process.env.NEXT_PUBLIC_API_BASE?.replace(/\/$/, '');
const apiRoute = (path: string) => (configuredBase ? `${configuredBase}${path}` : `*${path}`);

const FIXTURE_QUOTAS = [
  {
    profile: 'work',
    plan_tier: 'max20',
    window_5h: {
      used: 142,
      cap: 900,
      pct: 15.78,
      resets_at: new Date(Date.now() + 3_600_000).toISOString(),
    },
    window_weekly: {
      used: 1203,
      cap: 4500,
      pct: 26.73,
      resets_at: new Date(Date.now() + 24 * 3_600_000).toISOString(),
    },
  },
  {
    profile: 'personal',
    plan_tier: 'pro',
    window_5h: {
      used: 45,
      cap: 45,
      pct: 100,
      resets_at: new Date(Date.now() + 1_200_000).toISOString(),
    },
    window_weekly: {
      used: 0,
      cap: 0,
      pct: 0,
      resets_at: new Date(0).toISOString(),
    },
  },
];

export const handlers = [
  http.get(apiRoute('/api/health'), () =>
    HttpResponse.json({ ok: true, version: '0.1.0-dev-msw' }),
  ),

  http.get(apiRoute('/api/profiles'), () => HttpResponse.json(FIXTURE_PROFILES)),

  http.get(apiRoute('/api/usage'), ({ request }) => {
    const url = new URL(request.url);
    const profile = url.searchParams.get('profile') ?? undefined;
    const rows = generateUsageRows(profile);
    const total = aggregateTotal(rows);
    return HttpResponse.json({ rows, total });
  }),

  http.get(apiRoute('/api/usage/live'), () => {
    const stream = new ReadableStream({
      start(controller) {
        const rows = generateUsageRows();
        const payload = `event: usage\ndata: ${JSON.stringify(rows)}\n\n`;
        controller.enqueue(new TextEncoder().encode(payload));
        controller.close();
      },
    });
    return new HttpResponse(stream, {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      },
    });
  }),

  http.get(apiRoute('/api/recommendations/live'), () =>
    new HttpResponse('', {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      },
    }),
  ),

  http.get(apiRoute('/api/daemon/status'), () =>
    HttpResponse.json(FIXTURE_DAEMON_STATUS),
  ),

  http.get(apiRoute('/api/hooks/status'), ({ request }) => {
    const url = new URL(request.url);
    const profile = url.searchParams.get('profile');
    const rows = profile
      ? FIXTURE_HOOK_STATUS.filter((row) => row.profile === profile)
      : FIXTURE_HOOK_STATUS;
    return HttpResponse.json(rows);
  }),

  http.get(apiRoute('/api/sessions'), ({ request }) => {
    const url = new URL(request.url);
    const profile = url.searchParams.get('profile') ?? undefined;
    const status = url.searchParams.get('status');
    const rows = generateSessions(profile).filter((row) => !status || row.status === status);
    return HttpResponse.json(rows);
  }),

  http.get(apiRoute('/api/headroom'), () => HttpResponse.json(FIXTURE_HEADROOM)),

  http.get(apiRoute('/api/quota'), ({ request }) => {
    const url = new URL(request.url);
    const profile = url.searchParams.get('profile');
    const rows = profile
      ? FIXTURE_QUOTAS.filter((row) => row.profile === profile)
      : FIXTURE_QUOTAS;
    return HttpResponse.json(rows);
  }),
];
