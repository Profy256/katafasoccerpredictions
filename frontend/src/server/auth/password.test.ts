import { describe, expect, it } from 'vitest';
import vectors from './password-vectors.json';
import { DUMMY_PASSWORD, dummyHash, hashPassword, verifyPassword } from './password';

/**
 * The vectors were produced by the Go implementation (backend/tools/vectors)
 * before it was removed. Passing them is what makes this file safe to hold the
 * password hashes of every existing user.
 */

describe('argon2id compatibility with the Go implementation', () => {
  for (const vector of vectors.vectors) {
    it(`verifies the hash Go produced for ${JSON.stringify(vector.password)}`, async () => {
      await expect(verifyPassword(vector.password, vector.hash)).resolves.toBe(true);
    });
  }

  for (const [hash, wrongPassword] of Object.entries(vectors.wrongPassword)) {
    it(`rejects the wrong password for ${hash.slice(0, 24)}…`, async () => {
      await expect(verifyPassword(wrongPassword, hash)).resolves.toBe(false);
    });
  }

  for (const invalid of vectors.invalidHashes) {
    it(`treats ${JSON.stringify(invalid.slice(0, 32))} as a failed login`, async () => {
      await expect(verifyPassword('anything', invalid)).resolves.toBe(false);
    });
  }

  it('verifies the dummy hash Go generated', async () => {
    await expect(verifyPassword(vectors.dummyPassword, vectors.dummyHash)).resolves.toBe(true);
    expect(vectors.dummyPassword).toBe(DUMMY_PASSWORD);
  });

  it('builds a dummy hash of its own that verifies', async () => {
    const hash = await dummyHash();
    expect(hash).toMatch(/^\$argon2id\$v=19\$m=65536,t=3,p=4\$/);
    await expect(verifyPassword(DUMMY_PASSWORD, hash)).resolves.toBe(true);
  });

  it('round-trips a new password through hash and verify', async () => {
    const hash = await hashPassword('Uganda2026!');
    expect(hash).toMatch(/^\$argon2id\$v=19\$m=65536,t=3,p=4\$[A-Za-z0-9+/]+\$[A-Za-z0-9+/]+$/);
    await expect(verifyPassword('Uganda2026!', hash)).resolves.toBe(true);
    await expect(verifyPassword('Uganda2027!', hash)).resolves.toBe(false);
  });
});
