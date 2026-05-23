'use client';

import { useState } from 'react';
import { ChevronDown, Circle } from 'lucide-react';
import { ToggleTheme } from './toggle-theme';
import type { DaemonStatus } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export type LiveStatus = 'connected' | 'disconnected' | 'connecting';

export interface HeaderProfile {
  name: string;
  color?: string;
}

export interface HeaderProps {
  profiles: HeaderProfile[];
  selected: string | null;
  onSelect: (name: string | null) => void;
  live: LiveStatus;
  daemon?: DaemonStatus | null;
}

export function Header({ profiles, selected, onSelect, live, daemon }: HeaderProps) {
  const [open, setOpen] = useState(false);
  const selectedProfile = profiles.find((p) => p.name === selected) ?? null;
  const daemonText = daemon ? daemon.mode : 'foreground';
  const daemonState = daemon ? daemon.status : 'running';

  return (
    <header className="sticky top-0 z-20 flex items-center justify-between border-b border-card-border bg-background/80 px-6 py-3 backdrop-blur">
      <div className="flex items-center gap-3">
        <span className="font-mono text-lg font-semibold tracking-tight">ccx</span>
        <span className="text-xs text-muted">dashboard</span>
      </div>

      <div className="flex items-center gap-3">
        <span
          aria-label={`Daemon status ${daemonState}`}
          className="hidden items-center gap-1.5 rounded-md border border-card-border bg-card px-2 py-1 text-xs text-muted sm:inline-flex"
        >
          <Circle
            size={8}
            fill={
              daemonState === 'running'
                ? '#22c55e'
                : daemonState === 'starting'
                  ? '#f59e0b'
                  : '#71717a'
            }
            stroke="none"
          />
          <span>{daemonText}</span>
          <span className="text-muted/80">{daemonState}</span>
        </span>

        <div className="relative">
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-label="Filter by profile"
            aria-haspopup="menu"
            aria-expanded={open}
            className="inline-flex h-8 items-center gap-2 rounded-md border border-card-border bg-card px-3 text-sm hover:bg-grid"
          >
            {selectedProfile && (
              <span
                aria-hidden
                className="h-2 w-2 rounded-full"
                style={{ background: profileAccent(selectedProfile) }}
              />
            )}
            <span>{selectedProfile ? selectedProfile.name : 'All profiles'}</span>
            <ChevronDown size={14} />
          </button>
          {open && (
            <div
              role="menu"
              className="absolute right-0 mt-2 w-48 rounded-md border border-card-border bg-card py-1 shadow-lg"
            >
              <button
                role="menuitem"
                type="button"
                onClick={() => {
                  onSelect(null);
                  setOpen(false);
                }}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-grid"
              >
                <span className="h-2 w-2 rounded-full bg-muted" aria-hidden />
                All profiles
              </button>
              {profiles.map((p) => (
                <button
                  key={p.name}
                  role="menuitem"
                  type="button"
                  onClick={() => {
                    onSelect(p.name);
                    setOpen(false);
                  }}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-grid"
                >
                  <span
                    aria-hidden
                    className="h-2 w-2 rounded-full"
                    style={{ background: profileAccent(p) }}
                  />
                  {p.name}
                </button>
              ))}
            </div>
          )}
        </div>

        <span
          aria-label={
            live === 'connected'
              ? 'Live updates connected'
              : live === 'connecting'
                ? 'Live updates connecting'
                : 'Live updates disconnected'
          }
          className="inline-flex items-center gap-1.5 text-xs text-muted"
        >
          <Circle
            size={8}
            fill={
              live === 'connected'
                ? '#22c55e'
                : live === 'connecting'
                  ? '#f59e0b'
                  : '#71717a'
            }
            stroke="none"
          />
          {live === 'connected' ? 'live' : live === 'connecting' ? '...' : 'offline'}
        </span>

        <ToggleTheme />
      </div>
    </header>
  );
}
