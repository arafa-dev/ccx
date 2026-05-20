'use client';

import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { UsageRow } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface TimeSeriesProfileMeta {
  name: string;
  color?: string;
}

export interface TimeSeriesChartProps {
  usageRows: UsageRow[];
  profiles: TimeSeriesProfileMeta[];
}

interface DayPoint {
  day: string;
  [profileName: string]: number | string;
}

const compact = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 1,
});

export function TimeSeriesChart({ usageRows, profiles }: TimeSeriesChartProps) {
  const data = useMemo<DayPoint[]>(() => {
    if (usageRows.length === 0) return [];
    const byDay = new Map<string, DayPoint>();
    for (const row of usageRows) {
      const day = row.day.slice(0, 10);
      const point = byDay.get(day) ?? ({ day } as DayPoint);
      const total =
        row.usage.input_tokens +
        row.usage.output_tokens +
        row.usage.cache_read_tokens +
        row.usage.cache_create_tokens;
      const prev = (point[row.profile] as number | undefined) ?? 0;
      point[row.profile] = prev + total;
      byDay.set(day, point);
    }
    return Array.from(byDay.values()).sort((a, b) => a.day.localeCompare(b.day));
  }, [usageRows]);

  return (
    <section
      data-testid="time-series-chart"
      aria-label="Daily tokens by profile"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <h2 className="mb-3 text-sm font-medium">Daily tokens</h2>
      <div className="h-64">
        <ResponsiveContainer width="100%" height="100%" minWidth={1} minHeight={1}>
          <AreaChart data={data} margin={{ top: 10, right: 12, bottom: 0, left: 0 }}>
            <CartesianGrid stroke="var(--grid)" vertical={false} />
            <XAxis dataKey="day" stroke="var(--muted)" fontSize={11} tickMargin={6} />
            <YAxis
              stroke="var(--muted)"
              fontSize={11}
              tickFormatter={(v: number) => compact.format(v)}
              width={48}
            />
            <Tooltip
              contentStyle={{
                background: 'var(--card)',
                border: '1px solid var(--card-border)',
                borderRadius: 8,
                fontSize: 12,
              }}
              formatter={(v: number) => compact.format(v)}
            />
            {profiles.map((p) => (
              <Area
                key={p.name}
                type="monotone"
                dataKey={p.name}
                stackId="usage"
                stroke={profileAccent(p)}
                fill={profileAccent(p)}
                fillOpacity={0.25}
                isAnimationActive={false}
              />
            ))}
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
