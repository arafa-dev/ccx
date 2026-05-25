import type { components, paths } from './api-types';

export type Profile = components['schemas']['Profile'];
export type ProfileWithTotals = components['schemas']['ProfileWithTotals'];
export type Usage = components['schemas']['Usage'];
export type UsageRow = components['schemas']['UsageRow'];
export type UsageTotal = components['schemas']['UsageTotal'];
export type DaemonStatus = components['schemas']['DaemonStatus'];
export type HookStatus = components['schemas']['HookStatus'];
export type SessionTelemetry = components['schemas']['SessionTelemetry'];
export type HeadroomCandidate = components['schemas']['HeadroomCandidate'];
export type HeadroomResponse = components['schemas']['HeadroomResponse'];
export type QuotaWindow = components['schemas']['QuotaWindow'];
export type ProfileQuota = components['schemas']['ProfileQuota'];
export type RecommendationEvent = components['schemas']['RecommendationEvent'];

export interface HealthResponse {
  ok: boolean;
  version: string;
}

export interface UsageResponse {
  rows: UsageRow[];
  total: UsageTotal;
}

export interface GetUsageParams {
  profile?: string;
  project?: string;
  /** Duration like "24h", "7d", "30d". Default "24h" on the server. */
  since?: string;
}

export interface GetSessionsParams {
  profile?: string;
  status?: string;
  /** Duration like "24h", "7d", "30d". Omit for no time filter; default limit/newest-first applies. */
  since?: string;
  limit?: number;
}

/** API base URL. Reads NEXT_PUBLIC_API_BASE at build time, falls back to same-origin. */
export function apiBaseUrl(): string {
  const env =
    typeof process !== 'undefined'
      ? process.env.NEXT_PUBLIC_API_BASE
      : undefined;
  return (env && env.length > 0 ? env : '').replace(/\/$/, '');
}

async function getJSON<T>(path: string): Promise<T> {
  const url = `${apiBaseUrl()}${path}`;
  const res = await fetch(url, {
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(
      `ccx API ${res.status} on ${path}: ${body.slice(0, 200) || res.statusText}`,
    );
  }
  return (await res.json()) as T;
}

export async function getHealth(): Promise<HealthResponse> {
  return getJSON<HealthResponse>('/api/health');
}

export async function getProfiles(): Promise<ProfileWithTotals[]> {
  return getJSON<ProfileWithTotals[]>('/api/profiles');
}

export async function getUsage(params: GetUsageParams = {}): Promise<UsageResponse> {
  const qs = new URLSearchParams();
  if (params.profile) qs.set('profile', params.profile);
  if (params.project) qs.set('project', params.project);
  if (params.since) qs.set('since', params.since);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return getJSON<UsageResponse>(`/api/usage${suffix}`);
}

export async function getDaemonStatus(): Promise<DaemonStatus> {
  return getJSON<DaemonStatus>('/api/daemon/status');
}

export async function getHooksStatus(profile?: string): Promise<HookStatus[]> {
  const qs = new URLSearchParams();
  if (profile) qs.set('profile', profile);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return getJSON<HookStatus[]>(`/api/hooks/status${suffix}`);
}

export async function getSessions(
  params: GetSessionsParams = {},
): Promise<SessionTelemetry[]> {
  const qs = new URLSearchParams();
  if (params.profile) qs.set('profile', params.profile);
  if (params.status) qs.set('status', params.status);
  if (params.since) qs.set('since', params.since);
  if (params.limit) qs.set('limit', String(params.limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return getJSON<SessionTelemetry[]>(`/api/sessions${suffix}`);
}

export async function getHeadroom(): Promise<HeadroomResponse> {
  return getJSON<HeadroomResponse>('/api/headroom');
}

export async function getQuota(params: { profile?: string } = {}): Promise<ProfileQuota[]> {
  const qs = new URLSearchParams();
  if (params.profile) qs.set('profile', params.profile);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return getJSON<ProfileQuota[]>(`/api/quota${suffix}`);
}

/**
 * streamUsage opens an SSE connection to /api/usage/live and invokes onRows
 * for each emitted UsageRow array. Returns a teardown function.
 *
 * In Phase 1 this is mocked by MSW (which can simulate SSE via ReadableStream).
 * In production it talks to the Go server.
 */
export function streamUsage(
  onRows: (rows: UsageRow[]) => void,
  onError?: (err: Error) => void,
): () => void {
  const url = `${apiBaseUrl()}/api/usage/live`;
  const es = new EventSource(url);
  es.addEventListener('usage', (ev) => {
    try {
      const parsed = JSON.parse((ev as MessageEvent).data) as UsageRow[];
      onRows(parsed);
    } catch (e) {
      onError?.(e as Error);
    }
  });
  es.onerror = () => {
    onError?.(new Error('SSE connection error'));
  };
  return () => es.close();
}

/**
 * streamRecommendations opens an SSE connection to /api/recommendations/live
 * and invokes onEvent for each recommendation event. Malformed payloads are
 * ignored because the next SSE event may still be valid.
 */
export function streamRecommendations(
  onEvent: (event: RecommendationEvent) => void,
  onDisconnect: () => void,
): () => void {
  const url = `${apiBaseUrl()}/api/recommendations/live`;
  const es = new EventSource(url);
  es.addEventListener('recommendation', (ev) => {
    try {
      const parsed = JSON.parse((ev as MessageEvent).data) as RecommendationEvent;
      onEvent(parsed);
    } catch {
      // Ignore malformed SSE payloads.
    }
  });
  es.onerror = () => {
    onDisconnect();
  };
  return () => es.close();
}

// Compile-time check: ensure manual response shapes stay aligned with OpenAPI.
type _HealthCheck =
  paths['/api/health']['get']['responses']['200']['content']['application/json'];
type _ProfilesCheck =
  paths['/api/profiles']['get']['responses']['200']['content']['application/json'];
type _UsageCheck =
  paths['/api/usage']['get']['responses']['200']['content']['application/json'];
type _DaemonStatusCheck =
  paths['/api/daemon/status']['get']['responses']['200']['content']['application/json'];
type _HooksStatusCheck =
  paths['/api/hooks/status']['get']['responses']['200']['content']['application/json'];
type _SessionsCheck =
  paths['/api/sessions']['get']['responses']['200']['content']['application/json'];
type _HeadroomCheck =
  paths['/api/headroom']['get']['responses']['200']['content']['application/json'];
type _QuotaCheck =
  paths['/api/quota']['get']['responses']['200']['content']['application/json'];
type _RecommendationsLiveCheck =
  paths['/api/recommendations/live']['get']['responses']['200']['content']['text/event-stream'];
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _Assert = [
  _HealthCheck,
  _ProfilesCheck,
  _UsageCheck,
  _DaemonStatusCheck,
  _HooksStatusCheck,
  _SessionsCheck,
  _HeadroomCheck,
  _QuotaCheck,
  _RecommendationsLiveCheck,
];
