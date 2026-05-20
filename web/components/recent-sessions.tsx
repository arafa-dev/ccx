'use client';

import { useMemo } from 'react';
import type { UsageRow } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface RecentSessionsProps {
  usageRows: UsageRow[];
  profiles: { name: string; color?: string }[];
  limit?: number;
}

const usd = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
});
const dayFmt = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });

export function RecentSessions({ usageRows, profiles, limit = 20 }: RecentSessionsProps) {
  const sessions = useMemo(() => {
    const sorted = [...usageRows].sort((a, b) => b.day.localeCompare(a.day));
    return sorted.slice(0, limit);
  }, [usageRows, limit]);

  if (sessions.length === 0) {
    return (
      <section className="rounded-xl border border-card-border bg-card p-6 text-center text-sm text-muted">
        No sessions yet. Run <code className="font-mono">claude</code> to start
        tracking.
      </section>
    );
  }

  return (
    <section
      aria-label="Recent sessions"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <h2 className="mb-3 text-sm font-medium">Recent sessions</h2>
      <ul className="divide-y divide-card-border">
        {sessions.map((s, i) => {
          const meta = profiles.find((p) => p.name === s.profile) ?? { name: s.profile };
          return (
            <li
              key={`${s.profile}-${s.project}-${s.day}-${i}`}
              data-testid="session-row"
              className="flex items-center justify-between py-2"
            >
              <div className="flex items-center gap-3">
                <span
                  aria-hidden
                  className="h-2 w-2 rounded-full"
                  style={{ background: profileAccent(meta) }}
                />
                <span className="text-sm font-medium">{s.project ?? '-'}</span>
                <span className="text-xs text-muted">{s.profile}</span>
                {s.model && <span className="text-xs text-muted">{s.model}</span>}
              </div>
              <div className="flex items-center gap-4 font-mono text-xs tabular">
                <span className="text-muted">{dayFmt.format(new Date(s.day))}</span>
                <span>{usd.format(s.estimated_usd)}</span>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
