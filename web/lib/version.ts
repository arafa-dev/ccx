/** Build-time version. Replaced by `NEXT_PUBLIC_CCX_VERSION` env var if set. */
export const CCX_VERSION =
  (typeof process !== 'undefined' && process.env.NEXT_PUBLIC_CCX_VERSION) ||
  '0.1.0-dev';

export const CCX_REPO_URL = 'https://github.com/arafa-dev/ccx';
