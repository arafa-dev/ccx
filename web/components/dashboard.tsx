'use client';

import { useCallback, useEffect, useState } from 'react';
import { Footer } from './footer';
import { Header, type LiveStatus } from './header';
import { ProfileCards } from './profile-cards';
import { RecentSessions } from './recent-sessions';
import { TimeSeriesChart } from './time-series-chart';
import { TopProjects } from './top-projects';
import {
  getHealth,
  getProfiles,
  getUsage,
  streamUsage,
  type ProfileWithTotals,
  type UsageRow,
} from '@/lib/api';

export function Dashboard() {
  const [profiles, setProfiles] = useState<ProfileWithTotals[]>([]);
  const [usageRows, setUsageRows] = useState<UsageRow[]>([]);
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [live, setLive] = useState<LiveStatus>('connecting');
  const [refreshedAt, setRefreshedAt] = useState<Date>(new Date());
  const [loadError, setLoadError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [p, u] = await Promise.all([
        getProfiles(),
        getUsage({ profile: selectedProfile ?? undefined, since: '7d' }),
      ]);
      setProfiles(p);
      setUsageRows(u.rows);
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
            />
            <TimeSeriesChart usageRows={usageRows} profiles={profileMeta} />
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <TopProjects usageRows={usageRows} />
              <RecentSessions usageRows={usageRows} profiles={profileMeta} />
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
