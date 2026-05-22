import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Header } from './header';
import { ThemeProvider } from './theme-provider';

const profiles = [
  { name: 'work', color: '#3B82F6' },
  { name: 'personal', color: '#10B981' },
  { name: 'side', color: '#F59E0B' },
];

function renderHeader(props: Partial<React.ComponentProps<typeof Header>> = {}) {
  const onSelect = vi.fn();
  render(
    <ThemeProvider>
      <Header
        profiles={profiles}
        selected={null}
        onSelect={onSelect}
        live="connected"
        {...props}
      />
    </ThemeProvider>,
  );
  return { onSelect };
}

describe('<Header>', () => {
  it('renders the ccx wordmark', () => {
    renderHeader();
    expect(screen.getByText(/ccx/i)).toBeInTheDocument();
  });

  it('lists all profiles in the picker', async () => {
    renderHeader();
    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    expect(screen.getByRole('menuitem', { name: /all profiles/i })).toBeInTheDocument();
    for (const p of profiles) {
      expect(screen.getByRole('menuitem', { name: new RegExp(p.name, 'i') })).toBeInTheDocument();
    }
  });

  it('calls onSelect when a profile is chosen', async () => {
    const { onSelect } = renderHeader();
    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /work/i }));
    expect(onSelect).toHaveBeenCalledWith('work');
  });

  it('shows a green live-status dot when connected', () => {
    renderHeader({ live: 'connected' });
    expect(screen.getByLabelText(/live updates connected/i)).toBeInTheDocument();
  });

  it('shows a gray live-status dot when disconnected', () => {
    renderHeader({ live: 'disconnected' });
    expect(screen.getByLabelText(/live updates disconnected/i)).toBeInTheDocument();
  });

  it('shows daemon mode and status separately from live updates', () => {
    renderHeader({
      daemon: {
        mode: 'daemon',
        status: 'running',
        running: true,
        version: '0.1.0-test',
        profiles_watched: 3,
      },
    });
    expect(screen.getByLabelText(/daemon status running/i)).toHaveTextContent('daemon');
  });
});
