import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dashboard } from './dashboard';
import { ThemeProvider } from './theme-provider';
import type { ProfileWithTotals, UsageRow, UsageTotal } from '@/lib/api';

const profiles: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/x',
    color: '#3B82F6',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: blankTotal(),
  },
  {
    name: 'personal',
    config_dir: '/y',
    color: '#10B981',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: blankTotal(),
  },
];

function blankTotal(): UsageTotal {
  return {
    usage: {
      input_tokens: 0,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_create_tokens: 0,
    },
    estimated_usd: 0,
  };
}

function row(profile: string, project: string): UsageRow {
  return {
    profile,
    project,
    day: '2026-05-19T00:00:00Z',
    usage: {
      input_tokens: 100,
      output_tokens: 50,
      cache_read_tokens: 0,
      cache_create_tokens: 0,
    },
    session_count: 1,
    estimated_usd: 1,
  };
}

const allRows: UsageRow[] = [
  row('work', 'acme'),
  row('personal', 'hobby'),
  row('work', 'ccx'),
];

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<Record<string, unknown>>('@/lib/api');
  return {
    ...actual,
    getProfiles: vi.fn(async () => profiles),
    getUsage: vi.fn(async ({ profile }: { profile?: string } = {}) => {
      const rows = profile ? allRows.filter((r) => r.profile === profile) : allRows;
      return { rows, total: blankTotal() };
    }),
    getHealth: vi.fn(async () => ({ ok: true, version: '0.1.0-test' })),
    streamUsage: vi.fn(() => () => {}),
  };
});

describe('<Dashboard>', () => {
  beforeEach(() => vi.clearAllMocks());

  it('loads profiles and renders cards', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );
    await waitFor(() => {
      const profileRegion = screen.getByRole('region', { name: /profiles/i });
      expect(within(profileRegion).getByText('work')).toBeInTheDocument();
      expect(within(profileRegion).getByText('personal')).toBeInTheDocument();
    });
  });

  it('filters the projects table when picker changes', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    const topProjects = await screen.findByRole('region', { name: /top projects/i });
    await waitFor(() => expect(within(topProjects).getByText('acme')).toBeInTheDocument());
    expect(within(topProjects).getByText('hobby')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /^work$/i }));

    await waitFor(() => {
      expect(within(topProjects).queryByText('hobby')).not.toBeInTheDocument();
      expect(within(topProjects).getByText('acme')).toBeInTheDocument();
    });
  });
});
