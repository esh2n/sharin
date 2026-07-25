// Go 版 rate-limiter/ の4方式を TypeScript の純粋関数として移植したもの。
// Durable Object から呼ばれる。入力の state は変更せず、新しい state を返す。

// token bucket / leaky bucket 用
export const CAPACITY = 5;
export const RATE_PER_SEC = 0.5; // 補充または漏れの速度

// fixed window / sliding window log 用
export const LIMIT = 5;
export const WINDOW_MS = 10_000;

export const ALGOS = ["token-bucket", "leaky-bucket", "fixed-window", "sliding-window-log"] as const;
export type Algo = (typeof ALGOS)[number];

export function isAlgo(v: string): v is Algo {
  return (ALGOS as readonly string[]).includes(v);
}

export interface TokenBucketState {
  tokens: number;
  last: number;
}
export interface LeakyBucketState {
  water: number;
  last: number;
}
export interface FixedWindowState {
  start: number;
  count: number;
}
export interface SlidingLogState {
  log: number[];
}
export type AlgoState = TokenBucketState | LeakyBucketState | FixedWindowState | SlidingLogState;

export interface Outcome {
  allowed: boolean;
  remaining: number;
  retryAfterMs: number;
  state: AlgoState;
}

function takeTokenBucket(s: TokenBucketState | undefined, now: number): Outcome {
  const prev = s ?? { tokens: CAPACITY, last: now };
  const elapsedSec = Math.max(0, now - prev.last) / 1000;
  const tokens = Math.min(CAPACITY, prev.tokens + elapsedSec * RATE_PER_SEC);
  if (tokens < 1) {
    return {
      allowed: false,
      remaining: 0,
      retryAfterMs: Math.ceil(((1 - tokens) / RATE_PER_SEC) * 1000),
      state: { tokens, last: now },
    };
  }
  return {
    allowed: true,
    remaining: Math.floor(tokens - 1),
    retryAfterMs: 0,
    state: { tokens: tokens - 1, last: now },
  };
}

function takeLeakyBucket(s: LeakyBucketState | undefined, now: number): Outcome {
  const prev = s ?? { water: 0, last: now };
  const elapsedSec = Math.max(0, now - prev.last) / 1000;
  const water = Math.max(0, prev.water - elapsedSec * RATE_PER_SEC);
  if (water + 1 > CAPACITY) {
    return {
      allowed: false,
      remaining: 0,
      retryAfterMs: Math.ceil(((water + 1 - CAPACITY) / RATE_PER_SEC) * 1000),
      state: { water, last: now },
    };
  }
  return {
    allowed: true,
    remaining: Math.floor(CAPACITY - (water + 1)),
    retryAfterMs: 0,
    state: { water: water + 1, last: now },
  };
}

function takeFixedWindow(s: FixedWindowState | undefined, now: number): Outcome {
  const start = Math.floor(now / WINDOW_MS) * WINDOW_MS;
  const prev = s !== undefined && s.start === start ? s : { start, count: 0 };
  if (prev.count >= LIMIT) {
    return {
      allowed: false,
      remaining: 0,
      retryAfterMs: start + WINDOW_MS - now,
      state: prev,
    };
  }
  return {
    allowed: true,
    remaining: LIMIT - prev.count - 1,
    retryAfterMs: 0,
    state: { start, count: prev.count + 1 },
  };
}

function takeSlidingWindowLog(s: SlidingLogState | undefined, now: number): Outcome {
  const log = (s?.log ?? []).filter((t) => t > now - WINDOW_MS);
  if (log.length >= LIMIT) {
    return {
      allowed: false,
      remaining: 0,
      retryAfterMs: log[0] + WINDOW_MS - now,
      state: { log },
    };
  }
  return {
    allowed: true,
    remaining: LIMIT - log.length - 1,
    retryAfterMs: 0,
    state: { log: [...log, now] },
  };
}

export function apply(algo: Algo, state: AlgoState | undefined, now: number): Outcome {
  switch (algo) {
    case "token-bucket":
      return takeTokenBucket(state as TokenBucketState | undefined, now);
    case "leaky-bucket":
      return takeLeakyBucket(state as LeakyBucketState | undefined, now);
    case "fixed-window":
      return takeFixedWindow(state as FixedWindowState | undefined, now);
    case "sliding-window-log":
      return takeSlidingWindowLog(state as SlidingLogState | undefined, now);
  }
}
