'use client';

import { useMemo } from 'react';
import { ProfileCard } from './profile-card';
import type { HeadroomCandidate, HookStatus, ProfileWithTotals, UsageRow } from '@/lib/api';

export interface ProfileCardsProps {
  profiles: ProfileWithTotals[];
  usageRows: UsageRow[];
  candidates?: HeadroomCandidate[];
  hookStatuses?: HookStatus[];
}

export function ProfileCards({
  profiles,
  usageRows,
  candidates = [],
  hookStatuses = [],
}: ProfileCardsProps) {
  const sparklinesByProfile = useMemo(() => {
    const byProfile = new Map<string, Map<string, number>>();
    for (const row of usageRows) {
      const day = row.day.slice(0, 10);
      const inner = byProfile.get(row.profile) ?? new Map<string, number>();
      const total =
        row.usage.input_tokens +
        row.usage.output_tokens +
        row.usage.cache_read_tokens +
        row.usage.cache_create_tokens;
      inner.set(day, (inner.get(day) ?? 0) + total);
      byProfile.set(row.profile, inner);
    }
    const out = new Map<string, { day: string; tokens: number }[]>();
    for (const [profile, days] of byProfile) {
      const series = Array.from(days.entries())
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([day, tokens]) => ({ day, tokens }));
      out.set(profile, series);
    }
    return out;
  }, [usageRows]);

  const candidatesByProfile = useMemo(
    () => new Map(candidates.map((candidate) => [candidate.profile, candidate])),
    [candidates],
  );
  const hooksByProfile = useMemo(
    () => new Map(hookStatuses.map((status) => [status.profile, status])),
    [hookStatuses],
  );

  if (profiles.length === 0) return null;

  return (
    <section
      aria-label="Profiles"
      className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      {profiles.map((p) => (
        <ProfileCard
          key={p.name}
          profile={p}
          sparkline={sparklinesByProfile.get(p.name) ?? []}
          candidate={candidatesByProfile.get(p.name)}
          hookStatus={hooksByProfile.get(p.name)}
        />
      ))}
    </section>
  );
}
