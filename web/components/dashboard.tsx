'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { Footer } from './footer';
import { Header, type LiveStatus } from './header';
import { ProfileCards } from './profile-cards';
import { QuotaPanel } from './quota-panel';
import { RecentSessions } from './recent-sessions';
import { RecommendationPanel } from './recommendation-panel';
import { TimeSeriesChart } from './time-series-chart';
import { TopProjects } from './top-projects';
import {
  getDaemonStatus,
  getHeadroom,
  getHealth,
  getHooksStatus,
  getQuota,
  getProfiles,
  getSessions,
  getUsage,
  streamUsage,
  type DaemonStatus,
  type HeadroomResponse,
  type HookStatus,
  type ProfileWithTotals,
  type ProfileQuota,
  type SessionTelemetry,
  type UsageRow,
} from '@/lib/api';

function usageParams(profile: string | null) {
  return { profile: profile ?? undefined, since: '7d' };
}

function sessionParams(profile: string | null) {
  return profile ? { profile, since: '7d' } : { since: '7d' };
}

const LIVE_METADATA_REFRESH_INTERVAL_MS = 60_000;

export function Dashboard() {
  const [profiles, setProfiles] = useState<ProfileWithTotals[]>([]);
  const [usageRows, setUsageRows] = useState<UsageRow[]>([]);
  const [sessions, setSessions] = useState<SessionTelemetry[]>([]);
  const [daemonStatus, setDaemonStatus] = useState<DaemonStatus | null>(null);
  const [headroom, setHeadroom] = useState<HeadroomResponse | null>(null);
  const [hookStatuses, setHookStatuses] = useState<HookStatus[]>([]);
  const [quotas, setQuotas] = useState<ProfileQuota[]>([]);
  const [profilesLoaded, setProfilesLoaded] = useState(false);
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [live, setLive] = useState<LiveStatus>('connecting');
  const [refreshedAt, setRefreshedAt] = useState<Date>(new Date());
  const [loadError, setLoadError] = useState<string | null>(null);
  const selectedProfileRef = useRef<string | null>(null);
  const liveRefreshInFlight = useRef(false);
  const liveMetadataRefreshInFlight = useRef(false);
  const lastLiveMetadataRefreshAt = useRef(0);
  const profileRefreshToken = useRef(0);

  const refreshAll = useCallback(async (profile: string | null) => {
    try {
      const [p, u, d, h, sessionRows, hookRows, quotaRows] = await Promise.all([
        getProfiles(),
        getUsage(usageParams(profile)),
        getDaemonStatus(),
        getHeadroom(),
        getSessions(sessionParams(profile)),
        getHooksStatus(),
        getQuota(),
      ]);
      setProfiles(p);
      setProfilesLoaded(true);
      setDaemonStatus(d);
      setHeadroom(h);
      setHookStatuses(hookRows);
      setQuotas(quotaRows);
      if (selectedProfileRef.current === profile) {
        setUsageRows(u.rows);
        setSessions(sessionRows);
      }
      setRefreshedAt(new Date());
      setLoadError(null);
      lastLiveMetadataRefreshAt.current = Date.now();
    } catch (e) {
      if (selectedProfileRef.current !== profile) {
        return;
      }
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const refreshProfileData = useCallback(async (profile: string | null) => {
    const token = ++profileRefreshToken.current;
    try {
      const [p, u, sessionRows] = await Promise.all([
        getProfiles(),
        getUsage(usageParams(profile)),
        getSessions(sessionParams(profile)),
      ]);
      if (token !== profileRefreshToken.current || selectedProfileRef.current !== profile) {
        return;
      }
      setProfiles(p);
      setProfilesLoaded(true);
      setUsageRows(u.rows);
      setSessions(sessionRows);
      setRefreshedAt(new Date());
      setLoadError(null);
    } catch (e) {
      if (token !== profileRefreshToken.current || selectedProfileRef.current !== profile) {
        return;
      }
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const refreshLiveUsage = useCallback(async () => {
    if (liveRefreshInFlight.current) {
      return;
    }

    liveRefreshInFlight.current = true;
    const profile = selectedProfileRef.current;
    try {
      const [p, u] = await Promise.all([
        getProfiles(),
        getUsage(usageParams(profile)),
      ]);
      if (selectedProfileRef.current !== profile) {
        return;
      }
      setProfiles(p);
      setProfilesLoaded(true);
      setUsageRows(u.rows);
      setRefreshedAt(new Date());
      setLoadError(null);
    } catch (e) {
      if (selectedProfileRef.current !== profile) {
        return;
      }
      setLoadError(e instanceof Error ? e.message : String(e));
    } finally {
      liveRefreshInFlight.current = false;
    }
  }, []);

  const refreshLiveMetadata = useCallback(async () => {
    const now = Date.now();
    if (
      liveMetadataRefreshInFlight.current ||
      now - lastLiveMetadataRefreshAt.current < LIVE_METADATA_REFRESH_INTERVAL_MS
    ) {
      return;
    }

    liveMetadataRefreshInFlight.current = true;
    lastLiveMetadataRefreshAt.current = now;
    const profile = selectedProfileRef.current;
    try {
      const [d, h, sessionRows, hookRows, quotaRows] = await Promise.all([
        getDaemonStatus(),
        getHeadroom(),
        getSessions(sessionParams(profile)),
        getHooksStatus(),
        getQuota(),
      ]);
      setDaemonStatus(d);
      setHeadroom(h);
      setHookStatuses(hookRows);
      setQuotas(quotaRows);
      if (selectedProfileRef.current === profile) {
        setSessions(sessionRows);
        setLoadError(null);
      }
    } catch (e) {
      if (selectedProfileRef.current !== profile) {
        return;
      }
      setLoadError(e instanceof Error ? e.message : String(e));
    } finally {
      liveMetadataRefreshInFlight.current = false;
    }
  }, []);

  const handleSelectProfile = useCallback(
    (profile: string | null) => {
      selectedProfileRef.current = profile;
      setSelectedProfile(profile);
      void refreshProfileData(profile);
    },
    [refreshProfileData],
  );

  useEffect(() => {
    void refreshAll(null);
  }, [refreshAll]);

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
        void refreshLiveUsage();
        void refreshLiveMetadata();
      },
      () => setLive('disconnected'),
    );
    return stop;
  }, [refreshLiveUsage, refreshLiveMetadata]);

  const profileMeta = profiles.map((p) => ({ name: p.name, color: p.color }));
  const visibleQuotas = selectedProfile
    ? quotas.filter((quota) => quota.profile === selectedProfile)
    : quotas;

  return (
    <div className="mx-auto flex min-h-screen max-w-7xl flex-col">
      <Header
        profiles={profileMeta}
        selected={selectedProfile}
        onSelect={handleSelectProfile}
        live={live}
        daemon={daemonStatus}
      />

      <main className="flex flex-col gap-6 px-6 py-6">
        {loadError && (
          <div className="rounded-lg border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm">
            Failed to load: {loadError}
          </div>
        )}

        {!profilesLoaded && !loadError ? (
          <InitialLoading />
        ) : profilesLoaded && profiles.length === 0 && !loadError ? (
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
            <QuotaPanel quotas={visibleQuotas} />
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

function InitialLoading() {
  return (
    <section
      role="status"
      aria-label="Loading dashboard"
      className="rounded-xl border border-card-border bg-card p-6 text-sm text-muted"
    >
      Loading dashboard...
    </section>
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
