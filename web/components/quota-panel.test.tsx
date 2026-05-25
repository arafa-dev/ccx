import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import { QuotaPanel } from './quota-panel';
import type { ProfileQuota } from '@/lib/api';

const rows: ProfileQuota[] = [
  {
    profile: 'work',
    plan_tier: 'max20',
    window_5h: { used: 142, cap: 900, pct: 15.78, resets_at: '2026-05-24T19:00:00Z' },
    window_weekly: {
      used: 1203,
      cap: 4500,
      pct: 26.73,
      resets_at: '2026-05-31T00:00:00Z',
    },
  },
  {
    profile: 'personal',
    plan_tier: 'pro',
    window_5h: { used: 45, cap: 45, pct: 100, resets_at: '2026-05-24T18:50:00Z' },
    window_weekly: { used: 0, cap: 0, pct: 0, resets_at: '1970-01-01T00:00:00Z' },
  },
];

describe('QuotaPanel', () => {
  it('renders one row per profile', () => {
    render(<QuotaPanel quotas={rows} />);
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText('personal')).toBeInTheDocument();
  });

  it('shows used/cap labels for windows with non-zero cap', () => {
    render(<QuotaPanel quotas={rows} />);
    expect(screen.getByText(/142\s*\/\s*900/)).toBeInTheDocument();
    expect(screen.getByText(/1203\s*\/\s*4500/)).toBeInTheDocument();
  });

  it('shows em-dash for windows with cap=0', () => {
    render(<QuotaPanel quotas={rows} />);
    const row = screen.getByText('personal').closest('li');
    if (!row) throw new Error('expected personal row');
    expect(row.textContent).toMatch(/—/);
  });

  it('highlights profiles at hard cap', () => {
    render(<QuotaPanel quotas={rows} />);
    const meter = screen.getByRole('meter', { name: '5h quota for personal' });
    expect(meter).toHaveAttribute('aria-valuenow', '100');
    expect(meter).toHaveAttribute('aria-valuetext', '45 of 45 turns, 100%, at cap');
    expect(meter.querySelector('.absolute')).toHaveClass('bg-red-500');
  });

  it('uses raw percentages for threshold classes', () => {
    render(
      <QuotaPanel
        quotas={[
          {
            profile: 'near-hard',
            plan_tier: 'max20',
            window_5h: {
              used: 899,
              cap: 900,
              pct: 99.6,
              resets_at: '2026-05-24T19:00:00Z',
            },
            window_weekly: {
              used: 0,
              cap: 0,
              pct: 0,
              resets_at: '1970-01-01T00:00:00Z',
            },
          },
        ]}
      />,
    );
    const meter = screen.getByRole('meter', { name: '5h quota for near-hard' });
    const bar = meter.querySelector('.absolute');
    expect(bar).toHaveClass('bg-orange-500');
    expect(screen.queryByText(/⛔/)).not.toBeInTheDocument();
  });

  it('renders explicit "no plan tier" label for profiles with empty tier', () => {
    render(
      <QuotaPanel
        quotas={[
          {
            profile: 'api-dev',
            plan_tier: '',
            window_5h: {
              used: 0,
              cap: 0,
              pct: 0,
              resets_at: '1970-01-01T00:00:00Z',
            },
            window_weekly: {
              used: 0,
              cap: 0,
              pct: 0,
              resets_at: '1970-01-01T00:00:00Z',
            },
          },
        ]}
      />,
    );
    const row = screen.getByText('api-dev').closest('li');
    if (!row) throw new Error('expected api-dev row');
    expect(row.textContent).toMatch(/no plan tier/i);
    expect(row.textContent).toContain('—');
    expect(row.textContent).not.toMatch(/\d+\s*\/\s*\d+/);
  });
});
