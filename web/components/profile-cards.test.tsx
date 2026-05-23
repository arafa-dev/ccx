import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProfileCards } from './profile-cards';
import type { HeadroomCandidate, ProfileWithTotals, UsageRow } from '@/lib/api';

const profiles: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/x',
    color: '#3B82F6',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: {
      usage: {
        input_tokens: 1_000_000,
        output_tokens: 200_000,
        cache_read_tokens: 4_000_000,
        cache_create_tokens: 50_000,
      },
      estimated_usd: 12.34,
    },
  },
];

const rows: UsageRow[] = Array.from({ length: 7 }, (_, i) => ({
  profile: 'work',
  project: 'ccx',
  model: 'claude-opus-4-7',
  day: new Date(Date.UTC(2026, 4, 13 + i)).toISOString(),
  usage: {
    input_tokens: 1000 * (i + 1),
    output_tokens: 200 * (i + 1),
    cache_read_tokens: 0,
    cache_create_tokens: 0,
  },
  session_count: 1,
  estimated_usd: 0.5 * (i + 1),
}));

const candidates: HeadroomCandidate[] = [
  {
    profile: 'work',
    available: false,
    score: 42,
    headroom_percent: 12.5,
    auth_status: 'fail',
    cooldown_until: '2026-05-19T13:00:00Z',
    reasons: ['rate limit cooldown active'],
    priority: 0,
    tokens_24h: 90000,
    tokens_7d: 500000,
    usd_30d: 40,
  },
];

describe('<ProfileCards>', () => {
  it('renders one card per profile', () => {
    render(<ProfileCards profiles={profiles} usageRows={rows} />);
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText('$12.34')).toBeInTheDocument();
  });

  it('renders nothing gracefully when profile list is empty', () => {
    const { container } = render(<ProfileCards profiles={[]} usageRows={[]} />);
    expect(container.querySelectorAll('[data-testid="profile-card"]').length).toBe(0);
  });

  it('formats total tokens in compact notation', () => {
    render(<ProfileCards profiles={profiles} usageRows={rows} />);
    expect(screen.getByText(/5\.2[0-9]M/)).toBeInTheDocument();
  });

  it('shows availability, cooldown, and auth health from headroom candidates', () => {
    render(<ProfileCards profiles={profiles} usageRows={rows} candidates={candidates} />);
    expect(screen.getByText(/unavailable/i)).toBeInTheDocument();
    expect(screen.getByText(/cooldown/i)).toBeInTheDocument();
    expect(screen.getByText(/auth fail/i)).toBeInTheDocument();
  });
});
