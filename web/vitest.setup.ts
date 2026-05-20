import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// jsdom does not implement matchMedia; next-themes uses it.
if (typeof window !== 'undefined' && !window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// jsdom lacks ResizeObserver; Recharts uses it.
if (typeof window !== 'undefined' && !('ResizeObserver' in window)) {
  (window as unknown as { ResizeObserver: typeof ResizeObserver }).ResizeObserver =
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
}

if (typeof HTMLElement !== 'undefined') {
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return {
      x: 0,
      y: 0,
      width: 320,
      height: 160,
      top: 0,
      right: 320,
      bottom: 160,
      left: 0,
      toJSON: () => {},
    };
  };
}

afterEach(() => {
  cleanup();
});
