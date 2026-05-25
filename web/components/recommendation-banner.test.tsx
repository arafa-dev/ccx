import { describe, expect, it, vi, afterEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import { RecommendationBanner } from './recommendation-banner';
import { streamRecommendations, type RecommendationEvent } from '@/lib/api';

const baseEvent: RecommendationEvent = {
  profile: 'personal',
  level: 'soft',
  reason: '5h quota is above 80%',
  suggested: 'work',
  quota_5h_pct: 84.2,
  quota_weekly_pct: 24.5,
  timestamp: '2026-05-25T10:00:00Z',
};

describe('<RecommendationBanner>', () => {
  it('renders nothing when event is null', () => {
    const { container } = render(
      <RecommendationBanner event={null} onSwitch={vi.fn()} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('shows profile, level, reason, and suggested profile when present', () => {
    render(<RecommendationBanner event={baseEvent} onSwitch={vi.fn()} />);

    expect(screen.getByRole('status')).toBeInTheDocument();
    expect(screen.getByText('personal')).toBeInTheDocument();
    expect(screen.getByText(/soft/i)).toBeInTheDocument();
    expect(screen.getByText('5h quota is above 80%')).toBeInTheDocument();
    expect(screen.getByText('work', { selector: 'span' })).toBeInTheDocument();
  });

  it('renders hard-level banner with distinct marker text', () => {
    render(
      <RecommendationBanner
        event={{
          ...baseEvent,
          level: 'hard',
          reason: '5h quota has reached the hard cap',
        }}
        onSwitch={vi.fn()}
      />,
    );

    expect(screen.getByText('Hard limit')).toBeInTheDocument();
    expect(screen.getByText(/immediate action/i)).toBeInTheDocument();
  });

  it('hides switch button when no suggested sibling is present', () => {
    render(
      <RecommendationBanner event={{ ...baseEvent, suggested: '' }} onSwitch={vi.fn()} />,
    );

    expect(screen.queryByRole('button', { name: /switch/i })).not.toBeInTheDocument();
  });

  it('calls onSwitch with the suggested profile', () => {
    const onSwitch = vi.fn();
    render(<RecommendationBanner event={baseEvent} onSwitch={onSwitch} />);

    fireEvent.click(screen.getByRole('button', { name: /switch to work/i }));

    expect(onSwitch).toHaveBeenCalledWith('work');
  });
});

describe('streamRecommendations', () => {
  const originalEventSource = global.EventSource;

  afterEach(() => {
    global.EventSource = originalEventSource;
    vi.restoreAllMocks();
    vi.unstubAllEnvs();
  });

  it('opens recommendations SSE, emits parsed recommendation events, and closes', () => {
    vi.stubEnv('NEXT_PUBLIC_API_BASE', 'http://127.0.0.1:7777/');
    const sources: FakeEventSource[] = [];
    global.EventSource = class extends FakeEventSource {
      constructor(url: string | URL) {
        super(url);
        sources.push(this);
      }
    } as unknown as typeof EventSource;
    const onEvent = vi.fn();
    const onDisconnect = vi.fn();

    const teardown = streamRecommendations(onEvent, onDisconnect);
    sources[0]?.emit('recommendation', baseEvent);
    sources[0]?.emitRaw('recommendation', '{');
    sources[0]?.fail();
    teardown();

    expect(sources[0]?.url).toBe('http://127.0.0.1:7777/api/recommendations/live');
    expect(onEvent).toHaveBeenCalledOnce();
    expect(onEvent).toHaveBeenCalledWith(baseEvent);
    expect(onDisconnect).toHaveBeenCalledOnce();
    expect(sources[0]?.closed).toBe(true);
  });
});

class FakeEventSource {
  readonly url: string;
  closed = false;
  onerror: ((this: EventSource, ev: Event) => unknown) | null = null;
  private readonly listeners = new Map<string, Array<(event: MessageEvent) => void>>();

  constructor(url: string | URL) {
    this.url = String(url);
  }

  addEventListener(type: string, listener: EventListenerOrEventListenerObject): void {
    const fn =
      typeof listener === 'function'
        ? (listener as (event: MessageEvent) => void)
        : (event: MessageEvent) => listener.handleEvent(event);
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), fn]);
  }

  close(): void {
    this.closed = true;
  }

  emit(type: string, payload: RecommendationEvent): void {
    this.emitRaw(type, JSON.stringify(payload));
  }

  emitRaw(type: string, data: string): void {
    for (const listener of this.listeners.get(type) ?? []) {
      listener({ data } as MessageEvent);
    }
  }

  fail(): void {
    this.onerror?.call(this as unknown as EventSource, new Event('error'));
  }
}
