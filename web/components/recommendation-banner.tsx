'use client';

import { AlertTriangle, ArrowRight } from 'lucide-react';
import type { RecommendationEvent } from '@/lib/api';

export interface RecommendationBannerProps {
  event: RecommendationEvent | null;
  onSwitch: (toProfile: string) => void;
}

const LEVEL_LABELS: Record<RecommendationEvent['level'], string> = {
  warn: 'Warning',
  soft: 'Soft limit',
  hard: 'Hard limit',
};

export function RecommendationBanner({ event, onSwitch }: RecommendationBannerProps) {
  if (!event) return null;

  const suggested = event.suggested?.trim() ?? '';
  const isHard = event.level === 'hard';

  return (
    <section
      aria-live="polite"
      className={`rounded-xl border p-4 shadow-sm ${
        isHard
          ? 'border-red-300 bg-red-50 text-red-950 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-50'
          : 'border-card-border bg-card text-foreground'
      }`}
      role="status"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={`inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-semibold uppercase ${
                isHard ? 'bg-red-100 text-red-800 dark:bg-red-900/70 dark:text-red-100' : 'bg-grid text-muted'
              }`}
            >
              {isHard ? <AlertTriangle aria-hidden="true" className="h-3.5 w-3.5" /> : null}
              {LEVEL_LABELS[event.level]}
            </span>
            {isHard ? (
              <span className="text-xs font-semibold uppercase text-red-700 dark:text-red-200">
                Immediate action recommended
              </span>
            ) : null}
          </div>

          <div className="mt-3 flex flex-wrap items-baseline gap-2">
            <span className="text-sm text-muted">Profile</span>
            <span className="font-mono text-xl font-semibold tabular tracking-tight">
              {event.profile}
            </span>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6">{event.reason}</p>

          {suggested ? (
            <p className="mt-2 text-sm text-muted">
              Suggested profile:{' '}
              <span className="font-mono font-medium text-foreground">{suggested}</span>
            </p>
          ) : (
            <p className="mt-2 text-sm text-muted">No sibling profile has more headroom.</p>
          )}
        </div>

        {suggested ? (
          <button
            className={`inline-flex shrink-0 items-center justify-center gap-2 rounded-lg px-3 py-2 text-sm font-medium transition ${
              isHard
                ? 'bg-red-700 text-white hover:bg-red-800'
                : 'bg-foreground text-background hover:opacity-90'
            }`}
            onClick={() => onSwitch(suggested)}
            type="button"
          >
            Switch to {suggested}
            <ArrowRight aria-hidden="true" className="h-4 w-4" />
          </button>
        ) : null}
      </div>
    </section>
  );
}
