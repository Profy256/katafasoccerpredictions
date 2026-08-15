/**
 * Deterministic RNG so the demo dataset is identical on every render, on the
 * server and in the browser. Without this, server-rendered markup and client
 * hydration would disagree.
 */

/** mulberry32 — small, fast, good enough for generating sample fixtures. */
export function makeRng(seed: number): () => number {
  let a = seed >>> 0;
  return function next(): number {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** Knuth's method. Fine for the small lambdas involved in football scores. */
export function samplePoisson(lambda: number, rng: () => number): number {
  const limit = Math.exp(-lambda);
  let k = 0;
  let p = 1;
  do {
    k++;
    p *= rng();
  } while (p > limit);
  return k - 1;
}

/** Symmetric jitter in [-spread, +spread]. */
export function jitter(rng: () => number, spread: number): number {
  return (rng() * 2 - 1) * spread;
}
