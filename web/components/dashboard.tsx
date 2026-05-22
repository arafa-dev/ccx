'use client';

import { useCallback, useEffect, useState } from 'react';
import { Footer } from './footer';
import { Header, type LiveStatus } from './header';
import { ProfileCards } from './profile-cards';
import { RecentSessions } from './recent-sessions';
import { RecommendationPanel } from './recommendation-panel';
import { TimeSeriesChart } from './time-series-chart';
import { TopProjects } from './top-projects';
import {
  getDaemonStatus,
  getHeadroom,
  getHealth,
  getHooksStatus,
  getProfiles,
  getSessions,
  getUsage,
  streamUsage,
  type DaemonStatus,
  type HeadroomResponse,
  type HookStatus,
  type ProfileWithTotals,
  type SessionTelemetry,
  type UsageRow,
} from '@/lib/api';

export function Dashboard() {
  const [profiles, setProfiles] = useState<ProfileWithTotals[]>([]);
  const [usageRows, setUsageRows] = useState<UsageRow[]>([]);
  const [sessions, setSessions] = useState<SessionTelemetry[]>([]);
  const [daemonStatus, setDaemonStatus] = useState<DaemonStatus | null>(null);
  const [headroom, setHeadroom] = useState<HeadroomResponse | null>(null);
  const [hookStatuses, setHookStatuses] = useState<HookStatus[]>([]);
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [live, setLive] = useState<LiveStatus>('connecting');
  const [refreshedAt, setRefreshedAt] = useState<Date>(new Date());
  const [loadError, setLoadError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const usageParams = { profile: selectedProfile ?? undefined, since: '7d' };
      const sessionParams = selectedProfile
        ? { profile: selectedProfile, since: '7d' }
        : { since: '7d' };
      const [p, u, d, h, sessionRows, hookRows] = await Promise.all([
        getProfiles(),
        getUsage(usageParams),
        getDaemonStatus(),
        getHeadroom(),
        getSessions(sessionParams),
        getHooksStatus(),
      ]);
      setProfiles(p);
      setUsageRows(u.rows);
      setDaemonStatus(d);
      setHeadroom(h);
      setSessions(sessionRows);
      setHookStatuses(hookRows);
      setRefreshedAt(new Date());
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, [selectedProfile]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    let cancelled = false;
    void getHealth()
      .then(() => {
        if (!cancelled) setLive('connected');
      })
      .catch(() => {
        if (!cancelled) setLive('disconnected');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const stop = streamUsage(
      () => {
        setRefreshedAt(new Date());
        setLive('connected');
        void refresh();
      },
      () => setLive('disconnected'),
    );
    return stop;
  }, [refresh]);

  const profileMeta = profiles.map((p) => ({ name: p.name, color: p.color }));

  return (
    <div className="mx-auto flex min-h-screen max-w-7xl flex-col">
      <Header
        profiles={profileMeta}
        selected={selectedProfile}
        onSelect={setSelectedProfile}
        live={live}
        daemon={daemonStatus}
      />

      <main className="flex flex-col gap-6 px-6 py-6">
        {loadError && (
          <div className="rounded-lg border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm">
            Failed to load: {loadError}
          </div>
        )}

        {profiles.length === 0 && !loadError ? (
          <OnboardingEmpty />
        ) : (
          <>
            <ProfileCards
              profiles={
                selectedProfile ? profiles.filter((p) => p.name === selectedProfile) : profiles
              }
              usageRows={usageRows}
              candidates={headroom?.candidates ?? []}
              hookStatuses={hookStatuses}
            />
            <RecommendationPanel headroom={headroom} />
            <TimeSeriesChart usageRows={usageRows} profiles={profileMeta} />
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <TopProjects usageRows={usageRows} />
              <RecentSessions sessions={sessions} profiles={profileMeta} />
            </div>
          </>
        )}
      </main>

      <Footer lastRefreshed={refreshedAt} />
    </div>
  );
}

function OnboardingEmpty() {
  return (
    <section className="rounded-xl border border-card-border bg-card p-8 text-center">
      <h2 className="text-lg font-medium">No profiles yet</h2>
      <p className="mt-2 text-sm text-muted">
        Register your first Claude Code account to start tracking usage:
      </p>
      <pre className="mt-4 inline-block rounded-md bg-grid px-4 py-2 text-left font-mono text-xs">
        ccx profile add work --config-dir ~/.claude-profiles/work
      </pre>
    </section>
  );
}
