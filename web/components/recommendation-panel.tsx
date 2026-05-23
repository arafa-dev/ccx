'use client';

import type { HeadroomCandidate, HeadroomResponse } from '@/lib/api';

export interface RecommendationPanelProps {
  headroom: HeadroomResponse | null;
}

export function RecommendationPanel({ headroom }: RecommendationPanelProps) {
  const rec = headroom?.recommendation;
  if (!rec) {
    return (
      <section
        aria-label="Recommended profile"
        className="rounded-xl border border-card-border bg-card p-4"
      >
        <div className="text-sm font-medium">Recommended profile</div>
        <p className="mt-2 text-sm text-muted">No recommendable profile available.</p>
      </section>
    );
  }

  return (
    <section
      aria-label="Recommended profile"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-sm font-medium">Recommended profile</div>
          <div className="mt-1 flex items-baseline gap-3">
            <span className="font-mono text-2xl font-semibold tabular tracking-tight">
              {rec.profile}
            </span>
            <span className="text-xs text-muted">auth {rec.auth_status}</span>
          </div>
        </div>
        <div className="flex gap-4 text-right font-mono text-xs tabular">
          <Metric label="score" value={rec.score.toFixed(1)} />
          <Metric label="headroom" value={`${rec.headroom_percent.toFixed(1)}%`} />
        </div>
      </div>

      <ReasonList candidate={rec} />

      <code className="mt-3 inline-block rounded-md bg-grid px-3 py-2 font-mono text-xs">
        ccx use {rec.profile}
      </code>
    </section>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <span className="flex flex-col">
      <span className="text-muted">{label}</span>
      <span className="text-foreground">{value}</span>
    </span>
  );
}

function ReasonList({ candidate }: { candidate: HeadroomCandidate }) {
  const reasons = candidate.reasons.slice(0, 3);
  if (reasons.length === 0) return null;
  return (
    <ul className="mt-3 flex flex-wrap gap-2 text-xs text-muted">
      {reasons.map((reason) => (
        <li key={reason} className="rounded-md bg-grid px-2 py-1">
          {reason}
        </li>
      ))}
    </ul>
  );
}
