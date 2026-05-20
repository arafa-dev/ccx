import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';

const require = createRequire(import.meta.url);
const mswPackageDir = dirname(require.resolve('msw/package.json'));
const mswBrowserEntry = join(mswPackageDir, 'lib/browser/index.mjs');
const mswBrowserTurboEntry = './node_modules/msw/lib/browser/index.mjs';

/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  images: { unoptimized: true },
  trailingSlash: false,
  reactStrictMode: true,
  // The Go server (Phase 2) will serve these assets from /. The export needs
  // assetPrefix '' so URLs are relative.
  assetPrefix: '',
  // Disable Next.js telemetry in CI and dev builds for ccx.
  productionBrowserSourceMaps: false,
  experimental: {
    turbo: {
      resolveAlias: {
        'msw/browser': mswBrowserTurboEntry,
      },
    },
  },
  webpack: (config) => {
    config.resolve.alias = {
      ...config.resolve.alias,
      'msw/browser': mswBrowserEntry,
    };
    return config;
  },
};

export default nextConfig;
