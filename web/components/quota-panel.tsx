'use client';

import type { ProfileQuota, QuotaWindow } from '@/lib/api';

export interface QuotaPanelProps {
  quotas: ProfileQuota[];
}

export function QuotaPanel({ quotas }: QuotaPanelProps) {
  if (quotas.length === 0) return null;
  return (
    <section
      aria-label="Plan quota"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <h2 className="mb-3 text-sm font-medium uppercase text-muted">Plan Quota</h2>
      <ul className="space-y-3">
        {quotas.map((quota) => (
          <QuotaRow key={quota.profile} quota={quota} />
        ))}
      </ul>
    </section>
  );
}

function QuotaRow({ quota }: { quota: ProfileQuota }) {
  return (
    <li className="flex flex-col gap-3 rounded-lg bg-grid/40 p-3 sm:flex-row sm:items-center">
      <div className="min-w-0 sm:w-40 sm:shrink-0">
        <div className="truncate font-medium" title={quota.profile}>
          {quota.profile}
        </div>
        <div className="truncate text-xs text-muted">{quota.plan_tier || 'no plan tier'}</div>
      </div>
      <div className="min-w-0 flex-1 space-y-2">
        <QuotaBar label="5h" profile={quota.profile} window={quota.window_5h} />
        <QuotaBar label="7d" profile={quota.profile} window={quota.window_weekly} />
      </div>
    </li>
  );
}

function QuotaBar({
  label,
  profile,
  window,
}: {
  label: string;
  profile: string;
  window: QuotaWindow;
}) {
  if (window.cap === 0) {
    return (
      <div className="grid grid-cols-[2rem_minmax(0,1fr)_minmax(7rem,10rem)] items-center gap-2 text-xs text-muted">
        <span className="font-mono">{label}</span>
        <span aria-hidden="true" className="h-2 rounded-full bg-card-border" />
        <span className="whitespace-nowrap text-right font-mono">—</span>
      </div>
    );
  }

  const pct = Math.min(100, Math.max(0, window.pct));
  const capped = window.pct >= 100;
  return (
    <div
      aria-label={`${label} quota for ${profile}`}
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={Number(formatPct(pct))}
      aria-valuetext={`${window.used} of ${window.cap} turns, ${formatPct(window.pct)}%${capped ? ', at cap' : ''}`}
      className="grid grid-cols-[2rem_minmax(0,1fr)_minmax(7rem,10rem)] items-center gap-2"
      role="meter"
    >
      <span className="font-mono text-xs text-muted">{label}</span>
      <span className="relative h-2 overflow-hidden rounded-full bg-card-border">
        <span
          className={`absolute inset-y-0 left-0 ${barColor(pct)}`}
          style={{ width: `${pct}%` }}
        />
      </span>
      <span className="whitespace-nowrap text-right font-mono text-xs tabular">
        {window.used} / {window.cap} ({formatPct(window.pct)}%){capped ? ' ⛔' : ''}
      </span>
    </div>
  );
}

function formatPct(pct: number): string {
  const clamped = Math.min(100, Math.max(0, pct));
  if (Number.isInteger(clamped)) return String(clamped);
  return clamped.toFixed(1);
}

function barColor(pct: number): string {
  if (pct >= 100) return 'bg-red-500';
  if (pct >= 90) return 'bg-orange-500';
  if (pct >= 75) return 'bg-yellow-500';
  return 'bg-green-500';
}
