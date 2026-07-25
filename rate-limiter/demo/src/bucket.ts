// Go 版 rate-limiter/token_bucket.go と同じ lazy refill 方式の token bucket。
// Durable Object から呼ばれる純粋関数として実装し、状態の置き場所と計算を分離する。

export const CAPACITY = 5;
export const REFILL_PER_SEC = 0.5;

export interface BucketState {
  tokens: number;
  last: number; // 最後に残量を計算した時刻(epoch ms)
}

export interface TakeResult {
  allowed: boolean;
  remaining: number;
  retryAfterMs: number;
  state: BucketState;
}

// take は1トークンの消費を試みる。state が undefined なら満タンから始める。
// 入力の state は変更せず、新しい state を返す。
export function take(
  state: BucketState | undefined,
  now: number,
  capacity = CAPACITY,
  refillPerSec = REFILL_PER_SEC,
): TakeResult {
  const prev = state ?? { tokens: capacity, last: now };
  const elapsedSec = Math.max(0, now - prev.last) / 1000;
  const tokens = Math.min(capacity, prev.tokens + elapsedSec * refillPerSec);

  if (tokens < 1) {
    return {
      allowed: false,
      remaining: 0,
      retryAfterMs: Math.ceil(((1 - tokens) / refillPerSec) * 1000),
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
