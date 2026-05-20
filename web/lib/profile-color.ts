// Curated 8-color palette tuned for dark and light modes (Tailwind 500 shades).
const PALETTE = [
  '#3B82F6', // blue
  '#10B981', // emerald
  '#F59E0B', // amber
  '#EF4444', // red
  '#8B5CF6', // violet
  '#EC4899', // pink
  '#06B6D4', // cyan
  '#84CC16', // lime
] as const;

function hash(str: string): number {
  let h = 2166136261;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export interface ColorableProfile {
  name: string;
  color?: string;
}

/** Returns a stable accent color for the profile. Prefers profile.color if set. */
export function profileAccent(p: ColorableProfile): string {
  if (p.color && /^#[0-9A-Fa-f]{6}$/.test(p.color)) return p.color;
  return PALETTE[hash(p.name) % PALETTE.length]!;
}
