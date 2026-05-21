import { http, HttpResponse } from 'msw';
import { FIXTURE_PROFILES, aggregateTotal, generateUsageRows } from './fixtures';

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
];
