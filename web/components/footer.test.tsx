import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Footer } from './footer';

describe('<Footer>', () => {
  it('shows version and github link', () => {
    render(<Footer lastRefreshed={new Date('2026-05-19T12:00:00Z')} />);
    expect(screen.getByText(/v0\.1\.0/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /github/i })).toHaveAttribute(
      'href',
      'https://github.com/arafa-dev/ccx',
    );
  });

  it('shows a relative refreshed timestamp', () => {
    render(<Footer lastRefreshed={new Date(Date.now() - 5_000)} />);
    expect(screen.getByText(/refreshed/i)).toBeInTheDocument();
  });
});
