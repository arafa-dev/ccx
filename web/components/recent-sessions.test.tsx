import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecentSessions } from './recent-sessions';
import type { SessionTelemetry } from '@/lib/api';

const sessions: SessionTelemetry[] = Array.from({ length: 25 }, (_, i) => ({
  profile: i % 2 === 0 ? 'work' : 'personal',
  session: `session-${i}`,
  transcript: '',
  cwd: `/Users/arafa/projects/proj-${i}`,
  model: 'claude-opus-4-7',
  source: 'hook',
  permission: '',
  started_at: new Date(Date.UTC(2026, 4, 19, 10, 0 - i)).toISOString(),
  ended_at: new Date(Date.UTC(2026, 4, 19, 10, 5 - i)).toISOString(),
  last_seen_at: new Date(Date.UTC(2026, 4, 19, 10, 5 - i)).toISOString(),
  status: i === 0 ? 'failed' : 'completed',
  end_reason: i === 0 ? '' : 'stop',
  failure_error: i === 0 ? 'rate_limit' : '',
  failure_details: '',
  compact_count: i % 3,
}));
const firstSession = sessions[0]!;

describe('<RecentSessions>', () => {
  it('renders at most 20 rows', () => {
    render(<RecentSessions sessions={sessions} profiles={[{ name: 'work' }, { name: 'personal' }]} />);
    expect(screen.getAllByTestId('session-row').length).toBeLessThanOrEqual(20);
  });

  it('shows telemetry fields for profile, project, model, status, duration, and failure reason', () => {
    render(<RecentSessions sessions={sessions.slice(0, 1)} profiles={[{ name: 'work' }]} />);
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText('proj-0')).toBeInTheDocument();
    expect(screen.getByText('claude-opus-4-7')).toBeInTheDocument();
    expect(screen.getByText('failed')).toBeInTheDocument();
    expect(screen.getByText(/5m/)).toBeInTheDocument();
    expect(screen.getByText('rate_limit')).toBeInTheDocument();
  });

  it('renders a fallback when last_seen_at is invalid', () => {
    render(
      <RecentSessions
        sessions={[{ ...firstSession, last_seen_at: 'not-a-date' }]}
        profiles={[{ name: 'work' }]}
      />,
    );

    expect(screen.getByText('-')).toBeInTheDocument();
  });

  it('uses the final Windows path segment as the project label', () => {
    render(
      <RecentSessions
        sessions={[{ ...firstSession, cwd: 'C:\\Users\\arafa\\projects\\ccx' }]}
        profiles={[{ name: 'work' }]}
      />,
    );

    expect(screen.getByText('ccx')).toBeInTheDocument();
  });

  it('shows empty state when no rows', () => {
    render(<RecentSessions sessions={[]} profiles={[]} />);
    expect(screen.getByText(/no sessions yet/i)).toBeInTheDocument();
  });
});
