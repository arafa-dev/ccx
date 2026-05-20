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
};

export default nextConfig;
