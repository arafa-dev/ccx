'use client';

import { useMemo, useState } from 'react';
import { ArrowDown } from 'lucide-react';
import type { UsageRow } from '@/lib/api';

export interface TopProjectsProps {
  usageRows: UsageRow[];
  limit?: number;
}

type SortKey = 'tokens' | 'cost' | 'sessions';

interface Aggregated {
  project: string;
  tokens: number;
  cost: number;
  sessions: number;
}

const compact = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 2,
});
const usd = new Intl.NumberFormat('en-US', {
  style: 'currency',
  currency: 'USD',
});

export function TopProjects({ usageRows, limit = 10 }: TopProjectsProps) {
  const [sort, setSort] = useState<SortKey>('tokens');

  const aggregated = useMemo<Aggregated[]>(() => {
    const map = new Map<string, Aggregated>();
    for (const r of usageRows) {
      if (!r.project) continue;
      const total =
        r.usage.input_tokens +
        r.usage.output_tokens +
        r.usage.cache_read_tokens +
        r.usage.cache_create_tokens;
      const cur = map.get(r.project) ?? {
        project: r.project,
        tokens: 0,
        cost: 0,
        sessions: 0,
      };
      cur.tokens += total;
      cur.cost += r.estimated_usd;
      cur.sessions += r.session_count;
      map.set(r.project, cur);
    }
    const arr = Array.from(map.values());
    arr.sort((a, b) => b[sort] - a[sort]);
    return arr.slice(0, limit);
  }, [usageRows, sort, limit]);

  if (aggregated.length === 0) {
    return (
      <section className="rounded-xl border border-card-border bg-card p-6 text-center text-sm text-muted">
        No projects yet. Run <code className="font-mono">claude</code> to start
        tracking.
      </section>
    );
  }

  return (
    <section aria-label="Top projects" className="rounded-xl border border-card-border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium">Top projects</h2>
      </div>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wider text-muted">
            <th scope="col" className="px-2 py-2">
              Project
            </th>
            <SortableHeader
              label="Tokens"
              active={sort === 'tokens'}
              onClick={() => setSort('tokens')}
            />
            <SortableHeader
              label="Cost"
              active={sort === 'cost'}
              onClick={() => setSort('cost')}
            />
            <SortableHeader
              label="Sessions"
              active={sort === 'sessions'}
              onClick={() => setSort('sessions')}
            />
          </tr>
        </thead>
        <tbody>
          {aggregated.map((p) => (
            <tr key={p.project} className="border-t border-card-border">
              <td className="px-2 py-2 font-medium">{p.project}</td>
              <td className="px-2 py-2 text-right font-mono tabular">
                {compact.format(p.tokens)}
              </td>
              <td className="px-2 py-2 text-right font-mono tabular">
                {usd.format(p.cost)}
              </td>
              <td className="px-2 py-2 text-right font-mono tabular">{p.sessions}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function SortableHeader({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <th scope="col" className="px-2 py-2 text-right">
      <button
        type="button"
        onClick={onClick}
        className={`inline-flex items-center gap-1 ${
          active ? 'text-foreground' : 'text-muted hover:text-foreground'
        }`}
      >
        {label}
        {active && <ArrowDown size={12} />}
      </button>
    </th>
  );
}
