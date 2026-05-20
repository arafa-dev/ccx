import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { TimeSeriesChart } from './time-series-chart';
import type { UsageRow } from '@/lib/api';

describe('<TimeSeriesChart>', () => {
  it('renders without crashing on empty data', () => {
    const { container } = render(
      <TimeSeriesChart usageRows={[]} profiles={[{ name: 'work', color: '#3B82F6' }]} />,
    );
    expect(container.querySelector('[data-testid="time-series-chart"]')).toBeInTheDocument();
  });

  it('renders an SVG area element when data is provided', () => {
    const rows: UsageRow[] = [
      {
        profile: 'work',
        day: '2026-05-19T00:00:00Z',
        usage: {
          input_tokens: 1000,
          output_tokens: 500,
          cache_read_tokens: 0,
          cache_create_tokens: 0,
        },
        session_count: 1,
        estimated_usd: 0.5,
      },
      {
        profile: 'work',
        day: '2026-05-18T00:00:00Z',
        usage: {
          input_tokens: 1500,
          output_tokens: 700,
          cache_read_tokens: 0,
          cache_create_tokens: 0,
        },
        session_count: 1,
        estimated_usd: 0.8,
      },
    ];
    const { container } = render(
      <TimeSeriesChart usageRows={rows} profiles={[{ name: 'work', color: '#3B82F6' }]} />,
    );
    expect(container.querySelector('svg')).toBeInTheDocument();
  });
});
