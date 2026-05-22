import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecommendationPanel } from './recommendation-panel';
import type { HeadroomResponse } from '@/lib/api';

const headroom: HeadroomResponse = {
  recommendation: {
    profile: 'work',
    available: true,
    score: 96.4,
    headroom_percent: 88.2,
    auth_status: 'ok',
    reasons: ['daily tokens 1500/100000', 'auth ok'],
    priority: 10,
    tokens_24h: 1500,
    tokens_7d: 5000,
    usd_30d: 14.2,
  },
  candidates: [],
};

describe('<RecommendationPanel>', () => {
  it('renders recommended profile, score, reasons, and command', () => {
    render(<RecommendationPanel headroom={headroom} />);
    expect(screen.getByRole('region', { name: /recommended profile/i })).toBeInTheDocument();
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText(/96\.4/)).toBeInTheDocument();
    expect(screen.getByText(/88\.2%/)).toBeInTheDocument();
    expect(screen.getByText('daily tokens 1500/100000')).toBeInTheDocument();
    expect(screen.getByText('ccx use work')).toBeInTheDocument();
  });

  it('renders compact unavailable state when no recommendation exists', () => {
    render(<RecommendationPanel headroom={{ candidates: [] }} />);
    expect(screen.getByText(/no recommendable profile/i)).toBeInTheDocument();
  });
});
