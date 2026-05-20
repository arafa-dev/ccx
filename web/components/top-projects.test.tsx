import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TopProjects } from './top-projects';
import type { UsageRow } from '@/lib/api';

function row(project: string, tokens: number, usd: number, sessions: number): UsageRow {
  return {
    profile: 'work',
    project,
    day: '2026-05-19T00:00:00Z',
    usage: {
      input_tokens: tokens,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_create_tokens: 0,
    },
    session_count: sessions,
    estimated_usd: usd,
  };
}

const rows: UsageRow[] = [
  row('acme-api', 500_000, 8.5, 5),
  row('ccx', 1_200_000, 18.2, 12),
  row('hobby-site', 100_000, 1.1, 2),
];

describe('<TopProjects>', () => {
  it('renders rows sorted by tokens desc by default', () => {
    render(<TopProjects usageRows={rows} />);
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx');
  });

  it('sorts by cost when cost header clicked', async () => {
    render(<TopProjects usageRows={rows} />);
    await userEvent.click(screen.getByRole('button', { name: /cost/i }));
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx');
  });

  it('sorts by sessions when sessions header clicked', async () => {
    render(<TopProjects usageRows={rows} />);
    await userEvent.click(screen.getByRole('button', { name: /sessions/i }));
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx');
  });

  it('renders empty-state when no rows', () => {
    render(<TopProjects usageRows={[]} />);
    expect(screen.getByText(/no projects yet/i)).toBeInTheDocument();
  });
});
