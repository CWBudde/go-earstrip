// Numbers as the page writes them.

/** A duration in nanoseconds, to three digits. */
export function formatTime(ns: number): string {
  // The bounds sit where three digits round up to the next unit.
  if (ns < 999.5) return `${ns.toFixed(0)} ns`;
  if (ns < 999.5e3) return `${(ns / 1e3).toPrecision(3)} µs`;
  if (ns < 999.5e6) return `${(ns / 1e6).toPrecision(3)} ms`;
  return `${(ns / 1e9).toPrecision(3)} s`;
}

/** A duration in nanoseconds that is a power of ten, for axis ticks: 10 µs. */
export function formatTimeTick(ns: number): string {
  if (ns < 1e3) return `${ns} ns`;
  if (ns < 1e6) return `${ns / 1e3} µs`;
  if (ns < 1e9) return `${ns / 1e6} ms`;
  return `${ns / 1e9} s`;
}

/** A count, compact from 1,000 up: 100, 1K, 250K, 1M. */
export function formatCount(n: number): string {
  return new Intl.NumberFormat("en", { notation: "compact" }).format(n);
}
