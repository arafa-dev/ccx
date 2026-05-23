'use client';

import type { ReactNode } from 'react';
import { Line, LineChart, ResponsiveContainer } from 'recharts';
import type { HeadroomCandidate, HookStatus, ProfileWithTotals } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface ProfileCardProps {
  profile: ProfileWithTotals;
  sparkline: { day: string; tokens: number }[];
  candidate?: HeadroomCandidate;
  hookStatus?: HookStatus;
}

const compact = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 2,
});
const usd = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
});

export function ProfileCard({ profile, sparkline, candidate, hookStatus }: ProfileCardProps) {
  const accent = profileAccent(profile);
  const totalTokens =
    profile.today.usage.input_tokens +
    profile.today.usage.output_tokens +
    profile.today.usage.cache_read_tokens +
    profile.today.usage.cache_create_tokens;

  return (
    <div
      data-testid="profile-card"
      className="flex flex-col gap-3 rounded-xl border border-card-border bg-card p-4 transition hover:shadow-md"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span
            aria-hidden
            className="h-2.5 w-2.5 rounded-full"
            style={{ background: accent }}
          />
          <span className="text-sm font-medium">{profile.name}</span>
        </div>
        {profile.label && <span className="text-xs text-muted">{profile.label}</span>}
      </div>

      {(candidate || hookStatus) && (
        <div className="flex flex-wrap gap-1.5 text-xs">
          {candidate && (
            <>
              <StatusPill tone={candidate.available ? 'ok' : 'warn'}>
                {candidate.available ? 'available' : 'unavailable'}
              </StatusPill>
              <StatusPill tone={candidate.auth_status === 'ok' ? 'ok' : 'warn'}>
                auth {candidate.auth_status}
              </StatusPill>
              {candidate.cooldown_until && <StatusPill tone="warn">cooldown</StatusPill>}
            </>
          )}
          {hookStatus && (
            <StatusPill tone={hookStatus.installed && !hookStatus.disabled ? 'ok' : 'warn'}>
              hooks {hookStatus.status}
            </StatusPill>
          )}
        </div>
      )}

      <div className="flex items-baseline justify-between">
        <span className="font-mono text-2xl tabular tracking-tight">
          {usd.format(profile.today.estimated_usd)}
        </span>
        <span className="font-mono text-xs tabular text-muted">
          {compact.format(totalTokens)} tok
        </span>
      </div>

      <div className="-mx-1 h-10">
        <ResponsiveContainer width="100%" height="100%" minWidth={1} minHeight={1}>
          <LineChart data={sparkline} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
            <Line
              type="monotone"
              dataKey="tokens"
              stroke={accent}
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}

function StatusPill({
  children,
  tone,
}: {
  children: ReactNode;
  tone: 'ok' | 'warn';
}) {
  return (
    <span
      className={
        tone === 'ok'
          ? 'rounded-md bg-emerald-500/10 px-2 py-0.5 text-emerald-600 dark:text-emerald-300'
          : 'rounded-md bg-amber-500/10 px-2 py-0.5 text-amber-700 dark:text-amber-300'
      }
    >
      {children}
    </span>
  );
}
