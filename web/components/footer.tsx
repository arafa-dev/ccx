'use client';

import { Github } from 'lucide-react';
import { CCX_REPO_URL, CCX_VERSION } from '@/lib/version';

export interface FooterProps {
  lastRefreshed: Date;
}

function relative(date: Date): string {
  const diff = Math.max(0, (Date.now() - date.getTime()) / 1000);
  if (diff < 60) return `${Math.round(diff)}s ago`;
  if (diff < 3600) return `${Math.round(diff / 60)}m ago`;
  return `${Math.round(diff / 3600)}h ago`;
}

export function Footer({ lastRefreshed }: FooterProps) {
  return (
    <footer className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-card-border px-6 py-4 text-xs text-muted">
      <div className="flex items-center gap-3">
        <span className="font-mono">ccx v{CCX_VERSION}</span>
        <a
          href={CCX_REPO_URL}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 hover:text-foreground"
          aria-label="ccx on GitHub"
        >
          <Github size={12} />
          GitHub
        </a>
      </div>
      <span>refreshed {relative(lastRefreshed)}</span>
    </footer>
  );
}
