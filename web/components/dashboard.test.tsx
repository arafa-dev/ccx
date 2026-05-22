import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dashboard } from './dashboard';
import { ThemeProvider } from './theme-provider';
import {
  getDaemonStatus,
  getHeadroom,
  getHooksStatus,
  getProfiles,
  getSessions,
  getUsage,
  type HeadroomResponse,
  type HookStatus,
  type ProfileWithTotals,
  type SessionTelemetry,
  type UsageRow,
  type UsageTotal,
} from '@/lib/api';

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

const sessions: SessionTelemetry[] = [
  {
    profile: 'work',
    session: 's1',
    transcript: '',
    cwd: '/Users/arafa/projects/ccx',
    model: 'claude-opus-4-7',
    source: 'hook',
    permission: '',
    started_at: '2026-05-19T12:00:00Z',
    ended_at: '2026-05-19T12:05:00Z',
    last_seen_at: '2026-05-19T12:05:00Z',
    status: 'completed',
    end_reason: 'stop',
    failure_error: '',
    failure_details: '',
    compact_count: 0,
  },
];

const headroom: HeadroomResponse = {
  recommendation: {
    profile: 'work',
    available: true,
    score: 95,
    headroom_percent: 87,
    auth_status: 'ok',
    reasons: ['daily tokens 150/100000', 'auth ok'],
    priority: 5,
    tokens_24h: 150,
    tokens_7d: 1500,
    usd_30d: 3,
  },
  candidates: [
    {
      profile: 'work',
      available: true,
      score: 95,
      headroom_percent: 87,
      auth_status: 'ok',
      reasons: ['daily tokens 150/100000', 'auth ok'],
      priority: 5,
      tokens_24h: 150,
      tokens_7d: 1500,
      usd_30d: 3,
    },
  ],
};

const hooks: HookStatus[] = [
  {
    profile: 'work',
    installed: true,
    status: 'installed',
    disabled: false,
    settings_path: '/Users/arafa/.claude-profiles/work/settings.json',
  },
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
    getDaemonStatus: vi.fn(async () => ({
      mode: 'foreground',
      status: 'running',
      running: false,
      version: '0.1.0-test',
    })),
    getHeadroom: vi.fn(async () => headroom),
    getSessions: vi.fn(async () => sessions),
    getHooksStatus: vi.fn(async () => hooks),
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

  it('fetches daemon, headroom, sessions, hooks, profiles, and usage for the dashboard', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });
    expect(getDaemonStatus).toHaveBeenCalledOnce();
    expect(getHeadroom).toHaveBeenCalledOnce();
    expect(getSessions).toHaveBeenCalledWith({ since: '7d' });
    expect(getHooksStatus).toHaveBeenCalledOnce();
    expect(getProfiles).toHaveBeenCalledOnce();
    expect(getUsage).toHaveBeenCalledWith({ profile: undefined, since: '7d' });
  });

  it('renders daemon mode, recommendation, telemetry, and hook health', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    expect(await screen.findByLabelText(/daemon status running/i)).toHaveTextContent('foreground');
    expect(screen.getByRole('region', { name: /recommended profile/i })).toHaveTextContent('work');
    expect(screen.getByRole('region', { name: /session telemetry/i })).toHaveTextContent('ccx');
    expect(screen.getByText(/hooks installed/i)).toBeInTheDocument();
  });
});
