import type { ProfileWithTotals, UsageRow, UsageTotal } from '@/lib/api';

const now = Date.UTC(2026, 4, 19, 12, 0, 0);
const day = 24 * 60 * 60 * 1000;

function isoDay(offsetDays: number): string {
  return new Date(now - offsetDays * day).toISOString();
}

function usage(input: number, output: number, cacheR: number, cacheC: number) {
  return {
    input_tokens: input,
    output_tokens: output,
    cache_read_tokens: cacheR,
    cache_create_tokens: cacheC,
  };
}

function total(
  input: number,
  output: number,
  cacheR: number,
  cacheC: number,
  usd: number,
): UsageTotal {
  return { usage: usage(input, output, cacheR, cacheC), estimated_usd: usd };
}

export const FIXTURE_PROFILES: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/Users/arafa/.claude-profiles/work',
    label: 'Work account',
    color: '#3B82F6',
    created_at: '2026-04-01T10:00:00Z',
    last_used_at: isoDay(0),
    today: total(1_200_000, 240_000, 4_100_000, 60_000, 18.42),
  },
  {
    name: 'personal',
    config_dir: '/Users/arafa/.claude-profiles/personal',
    label: 'Personal',
    color: '#10B981',
    created_at: '2026-04-05T10:00:00Z',
    last_used_at: isoDay(0),
    today: total(220_000, 84_000, 980_000, 12_000, 4.18),
  },
  {
    name: 'side',
    config_dir: '/Users/arafa/.claude-profiles/side',
    label: 'Side project',
    color: '#F59E0B',
    created_at: '2026-05-10T10:00:00Z',
    last_used_at: isoDay(1),
    today: total(80_000, 30_000, 200_000, 5_000, 1.12),
  },
];

const PROJECTS = ['ccx', 'acme-api', 'hobby-site', 'experiments', 'devops'];

export function generateUsageRows(profileFilter?: string): UsageRow[] {
  const rows: UsageRow[] = [];
  for (const profile of FIXTURE_PROFILES) {
    if (profileFilter && profile.name !== profileFilter) continue;
    for (let d = 6; d >= 0; d--) {
      for (let p = 0; p < 2; p++) {
        const project =
          PROJECTS[(d + p + profile.name.length) % PROJECTS.length]!;
        const scale =
          profile.name === 'work'
            ? 1.0
            : profile.name === 'personal'
              ? 0.35
              : 0.15;
        const input = Math.round((80_000 + Math.sin(d + p) * 30_000) * scale);
        const output = Math.round((20_000 + Math.cos(d + p) * 8_000) * scale);
        const cacheR = Math.round(input * 3.5);
        const cacheC = Math.round(input * 0.05);
        const usd =
          (input / 1_000_000) * 3 +
          (output / 1_000_000) * 15 +
          (cacheR / 1_000_000) * 0.3 +
          (cacheC / 1_000_000) * 3.75;
        rows.push({
          profile: profile.name,
          project,
          model: d % 2 === 0 ? 'claude-opus-4-7' : 'claude-sonnet-4-6',
          day: isoDay(d),
          usage: usage(input, output, cacheR, cacheC),
          session_count: 1 + ((d + p) % 4),
          estimated_usd: Number(usd.toFixed(2)),
        });
      }
    }
  }
  return rows;
}

export function aggregateTotal(rows: UsageRow[]): UsageTotal {
  const usd = rows.reduce((s, r) => s + r.estimated_usd, 0);
  const u = rows.reduce(
    (s, r) => ({
      input_tokens: s.input_tokens + r.usage.input_tokens,
      output_tokens: s.output_tokens + r.usage.output_tokens,
      cache_read_tokens: s.cache_read_tokens + r.usage.cache_read_tokens,
      cache_create_tokens: s.cache_create_tokens + r.usage.cache_create_tokens,
    }),
    {
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_create_tokens: 0,
    },
  );
  return { usage: u, estimated_usd: Number(usd.toFixed(2)) };
}
