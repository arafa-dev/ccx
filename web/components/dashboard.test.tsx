import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dashboard } from './dashboard';
import { ThemeProvider } from './theme-provider';
import {
  getDaemonStatus,
  getHealth,
  getHeadroom,
  getHooksStatus,
  getQuota,
  getProfiles,
  getSessions,
  getUsage,
  streamRecommendations,
  streamUsage,
  type HeadroomResponse,
  type HookStatus,
  type ProfileWithTotals,
  type ProfileQuota,
  type RecommendationEvent,
  type SessionTelemetry,
  type UsageResponse,
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

const quotas: ProfileQuota[] = [
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
    getQuota: vi.fn(async () => quotas),
    streamRecommendations: vi.fn(() => () => {}),
    streamUsage: vi.fn(() => () => {}),
  };
});

describe('<Dashboard>', () => {
  beforeEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
    vi.mocked(getProfiles).mockImplementation(async () => profiles);
    vi.mocked(getUsage).mockImplementation(async ({ profile }: { profile?: string } = {}) => {
      const rows = profile ? allRows.filter((r) => r.profile === profile) : allRows;
      return { rows, total: blankTotal() };
    });
    vi.mocked(getHealth).mockImplementation(async () => ({ ok: true, version: '0.1.0-test' }));
    vi.mocked(getDaemonStatus).mockImplementation(async () => ({
      mode: 'foreground',
      status: 'running',
      running: false,
      version: '0.1.0-test',
    }));
    vi.mocked(getHeadroom).mockImplementation(async () => headroom);
    vi.mocked(getSessions).mockImplementation(async () => sessions);
    vi.mocked(getHooksStatus).mockImplementation(async () => hooks);
    vi.mocked(getQuota).mockImplementation(async () => quotas);
    vi.mocked(streamRecommendations).mockImplementation(() => () => {});
    vi.mocked(streamUsage).mockImplementation(() => () => {});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows loading state during the initial dashboard load instead of onboarding empty', async () => {
    let resolveProfiles: (value: ProfileWithTotals[]) => void = () => {};
    vi.mocked(getProfiles).mockImplementationOnce(
      () =>
        new Promise<ProfileWithTotals[]>((resolve) => {
          resolveProfiles = resolve;
        }),
    );

    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    expect(screen.getByRole('status', { name: /loading dashboard/i })).toBeInTheDocument();
    expect(screen.queryByText(/no profiles yet/i)).not.toBeInTheDocument();

    resolveProfiles([]);

    expect(await screen.findByText(/no profiles yet/i)).toBeInTheDocument();
  });

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

  it('fetches daemon, headroom, sessions, hooks, quota, profiles, and usage for the dashboard', async () => {
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
    expect(getQuota).toHaveBeenCalledOnce();
    expect(getProfiles).toHaveBeenCalledOnce();
    expect(getUsage).toHaveBeenCalledWith({ profile: undefined, since: '7d' });
  });

  it('keeps expensive metadata refreshes out of live usage SSE ticks', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });
    const onUsage = vi.mocked(streamUsage).mock.calls[0]?.[0];
    expect(onUsage).toBeDefined();

    await act(async () => {
      onUsage?.([]);
      onUsage?.([]);
      onUsage?.([]);
    });

    await waitFor(() => expect(getUsage).toHaveBeenCalledTimes(2));
    expect(getProfiles).toHaveBeenCalledTimes(2);
    expect(getDaemonStatus).toHaveBeenCalledOnce();
    expect(getHeadroom).toHaveBeenCalledOnce();
    expect(getHooksStatus).toHaveBeenCalledOnce();
    expect(getSessions).toHaveBeenCalledOnce();
    expect(getQuota).toHaveBeenCalledOnce();
  });

  it('throttles expensive metadata refreshes during live usage SSE ticks', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    vi.setSystemTime(new Date('2026-05-19T12:00:00Z'));

    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });
    const onUsage = vi.mocked(streamUsage).mock.calls[0]?.[0];
    expect(onUsage).toBeDefined();

    vi.mocked(getDaemonStatus).mockClear();
    vi.mocked(getHeadroom).mockClear();
    vi.mocked(getHooksStatus).mockClear();
    vi.mocked(getSessions).mockClear();
    vi.mocked(getQuota).mockClear();

    await act(async () => {
      onUsage?.([]);
    });

    expect(getDaemonStatus).not.toHaveBeenCalled();
    expect(getHeadroom).not.toHaveBeenCalled();
    expect(getHooksStatus).not.toHaveBeenCalled();
    expect(getSessions).not.toHaveBeenCalled();
    expect(getQuota).not.toHaveBeenCalled();

    await act(async () => {
      vi.setSystemTime(new Date('2026-05-19T12:01:01Z'));
      onUsage?.([]);
    });

    await waitFor(() => expect(getDaemonStatus).toHaveBeenCalledOnce());
    expect(getHeadroom).toHaveBeenCalledOnce();
    expect(getHooksStatus).toHaveBeenCalledOnce();
    expect(getSessions).toHaveBeenCalledWith({ since: '7d' });
    expect(getQuota).toHaveBeenCalledOnce();
  });

  it('skips overlapping live usage refreshes while one is in flight', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });
    const onUsage = vi.mocked(streamUsage).mock.calls[0]?.[0];
    expect(onUsage).toBeDefined();

    let resolveLiveUsage: (value: UsageResponse) => void = () => {};
    vi.mocked(getUsage).mockImplementation(
      () =>
        new Promise<UsageResponse>((resolve) => {
          resolveLiveUsage = resolve;
        }),
    );
    vi.mocked(getProfiles).mockClear();
    vi.mocked(getUsage).mockClear();

    await act(async () => {
      onUsage?.([]);
      onUsage?.([]);
    });

    expect(getProfiles).toHaveBeenCalledOnce();
    expect(getUsage).toHaveBeenCalledOnce();

    await act(async () => {
      resolveLiveUsage({ rows: allRows, total: blankTotal() });
    });
  });

  it('ignores stale profile refresh errors after selecting another profile', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });

    const originalGetUsage = vi.mocked(getUsage).getMockImplementation();
    let rejectWorkUsage: (reason: Error) => void = () => {};
    vi.mocked(getUsage).mockImplementation(({ profile }: { profile?: string } = {}) => {
      if (profile === 'work') {
        return new Promise<UsageResponse>((_, reject) => {
          rejectWorkUsage = reject;
        });
      }
      return originalGetUsage?.({ profile }) ?? Promise.resolve({ rows: [], total: blankTotal() });
    });

    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /^work$/i }));
    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /^personal$/i }));

    await waitFor(() => {
      const topProjects = screen.getByRole('region', { name: /top projects/i });
      expect(within(topProjects).getByText('hobby')).toBeInTheDocument();
    });

    await act(async () => {
      rejectWorkUsage(new Error('stale work failure'));
    });

    expect(screen.queryByText(/stale work failure/i)).not.toBeInTheDocument();
  });

  it('renders daemon mode, recommendation, telemetry, and hook health', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    expect(await screen.findByLabelText(/daemon status running/i)).toHaveTextContent('foreground');
    expect(await screen.findByRole('region', { name: /recommended profile/i })).toHaveTextContent(
      'work',
    );
    expect(await screen.findByRole('region', { name: /session telemetry/i })).toHaveTextContent(
      'ccx',
    );
    expect(screen.getByText(/hooks installed/i)).toBeInTheDocument();
  });

  it('renders the quota panel when quotas are loaded', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    expect(await screen.findByText(/plan quota/i)).toBeInTheDocument();
  });

  it('renders live recommendation events and switches to the suggested profile', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await screen.findByRole('region', { name: /recommended profile/i });
    const onRecommendation = vi.mocked(streamRecommendations).mock.calls[0]?.[0];
    expect(onRecommendation).toBeDefined();

    const event: RecommendationEvent = {
      profile: 'personal',
      level: 'hard',
      reason: '5h quota has reached the hard cap',
      suggested: 'work',
      quota_5h_pct: 100,
      quota_weekly_pct: 42,
      timestamp: '2026-05-25T10:00:00Z',
    };

    await act(async () => {
      onRecommendation?.(event);
    });

    const banner = screen.getByRole('status');
    expect(within(banner).getByText('Hard limit')).toBeInTheDocument();
    expect(within(banner).getByText('personal')).toBeInTheDocument();
    expect(within(banner).getByText('5h quota has reached the hard cap')).toBeInTheDocument();

    await userEvent.click(within(banner).getByRole('button', { name: /switch to work/i }));

    await waitFor(() => {
      const topProjects = screen.getByRole('region', { name: /top projects/i });
      expect(within(topProjects).getByText('acme')).toBeInTheDocument();
      expect(within(topProjects).queryByText('hobby')).not.toBeInTheDocument();
    });
  });

  it('does not let recommendation stream disconnects change the usage live indicator', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    expect(await screen.findByLabelText(/live updates connected/i)).toBeInTheDocument();
    const onRecommendationDisconnect = vi.mocked(streamRecommendations).mock.calls[0]?.[1];
    expect(onRecommendationDisconnect).toBeDefined();

    await act(async () => {
      onRecommendationDisconnect?.();
    });

    expect(screen.getByLabelText(/live updates connected/i)).toBeInTheDocument();
  });

  it('filters the quota panel when picker changes', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    const quotaPanel = await screen.findByRole('region', { name: /plan quota/i });
    await waitFor(() => expect(within(quotaPanel).getByText('personal')).toBeInTheDocument());

    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /^work$/i }));

    await waitFor(() => {
      expect(within(quotaPanel).getByText('work')).toBeInTheDocument();
      expect(within(quotaPanel).queryByText('personal')).not.toBeInTheDocument();
    });
  });
});
