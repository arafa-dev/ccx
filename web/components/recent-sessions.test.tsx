import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecentSessions } from './recent-sessions';
import type { UsageRow } from '@/lib/api';

const rows: UsageRow[] = Array.from({ length: 25 }, (_, i) => ({
  profile: i % 2 === 0 ? 'work' : 'personal',
  project: `proj-${i}`,
  model: 'claude-opus-4-7',
  day: new Date(Date.UTC(2026, 4, 19 - i)).toISOString(),
  usage: {
    input_tokens: 100 * i,
    output_tokens: 50,
    cache_read_tokens: 0,
    cache_create_tokens: 0,
  },
  session_count: 1,
  estimated_usd: 0.5,
}));

describe('<RecentSessions>', () => {
  it('renders at most 20 rows', () => {
    render(<RecentSessions usageRows={rows} profiles={[{ name: 'work' }, { name: 'personal' }]} />);
    expect(screen.getAllByTestId('session-row').length).toBeLessThanOrEqual(20);
  });

  it('shows empty state when no rows', () => {
    render(<RecentSessions usageRows={[]} profiles={[]} />);
    expect(screen.getByText(/no sessions yet/i)).toBeInTheDocument();
  });
});
