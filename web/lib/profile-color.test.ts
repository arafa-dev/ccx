import { describe, it, expect } from 'vitest';
import { profileAccent } from './profile-color';

describe('profileAccent', () => {
  it('returns the profile color when set', () => {
    expect(profileAccent({ name: 'work', color: '#3B82F6' })).toBe('#3B82F6');
  });

  it('returns a stable hex for the same name when color is missing', () => {
    const a = profileAccent({ name: 'work' });
    const b = profileAccent({ name: 'work' });
    expect(a).toBe(b);
    expect(a).toMatch(/^#[0-9A-Fa-f]{6}$/);
  });

  it('returns different colors for different names', () => {
    expect(profileAccent({ name: 'work' })).not.toBe(
      profileAccent({ name: 'personal' }),
    );
  });
});
