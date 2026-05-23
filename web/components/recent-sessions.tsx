'use client';

import { useMemo } from 'react';
import type { SessionTelemetry } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface RecentSessionsProps {
  sessions: SessionTelemetry[];
  profiles: { name: string; color?: string }[];
  limit?: number;
}

const timeFmt = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  hour: 'numeric',
  minute: '2-digit',
});

export function RecentSessions({ sessions, profiles, limit = 20 }: RecentSessionsProps) {
  const visibleSessions = useMemo(() => {
    const sorted = [...sessions].sort((a, b) => b.last_seen_at.localeCompare(a.last_seen_at));
    return sorted.slice(0, limit);
  }, [sessions, limit]);

  if (visibleSessions.length === 0) {
    return (
      <section className="rounded-xl border border-card-border bg-card p-6 text-center text-sm text-muted">
        No sessions yet. Run <code className="font-mono">claude</code> to start
        tracking.
      </section>
    );
  }

  return (
    <section
      aria-label="Session telemetry"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium">Session telemetry</h2>
        <span className="font-mono text-xs tabular text-muted">
          {visibleSessions.length} recent
        </span>
      </div>
      <ul className="divide-y divide-card-border">
        {visibleSessions.map((s) => {
          const meta = profiles.find((p) => p.name === s.profile) ?? { name: s.profile };
          const duration = durationLabel(s);
          return (
            <li
              key={`${s.profile}-${s.session}`}
              data-testid="session-row"
              className="grid grid-cols-[1fr_auto] gap-3 py-2"
            >
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
                  <span
                    aria-hidden
                    className="h-2 w-2 rounded-full"
                    style={{ background: profileAccent(meta) }}
                  />
                  <span className="truncate text-sm font-medium">{projectLabel(s.cwd)}</span>
                  <span className="text-xs text-muted">{s.profile}</span>
                  {s.model && <span className="text-xs text-muted">{s.model}</span>}
                </div>
                <div className="mt-1 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted">
                  <span>{s.status}</span>
                  {duration && <span>{duration}</span>}
                  {s.end_reason && <span>{s.end_reason}</span>}
                  {s.failure_error && <span>{s.failure_error}</span>}
                  {s.compact_count > 0 && <span>{s.compact_count} compact</span>}
                </div>
              </div>
              <div className="whitespace-nowrap font-mono text-xs tabular text-muted">
                {formatLastSeen(s.last_seen_at)}
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}

function projectLabel(cwd: string): string {
  const parts = cwd.split(/[\\/]+/).filter(Boolean);
  return parts.at(-1) ?? (cwd || '-');
}

function formatLastSeen(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) {
    return '-';
  }
  return timeFmt.format(date);
}

function durationLabel(session: SessionTelemetry): string | null {
  const start = Date.parse(session.started_at);
  const end = Date.parse(session.ended_at);
  if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) {
    return null;
  }
  const minutes = Math.round((end - start) / 60000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`;
}
