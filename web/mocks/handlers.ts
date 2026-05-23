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
];
