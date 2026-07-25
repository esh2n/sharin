import { describe, expect, it } from "vitest";
import { type BucketState, take } from "./bucket";

const CAP = 5;
const RATE = 0.5; // tokens per second

// state を undefined から始めて、(経過ms, 期待) のステップを順に流すヘルパー。
function run(steps: Array<{ afterMs: number; want: boolean }>) {
  let state: BucketState | undefined;
  let now = 1_000_000;
  const got: boolean[] = [];
  for (const s of steps) {
    now += s.afterMs;
    const r = take(state, now, CAP, RATE);
    state = r.state;
    got.push(r.allowed);
  }
  return { got, want: steps.map((s) => s.want) };
}

describe("take", () => {
  it("初期状態は満タンで容量分のバーストを許す", () => {
    const { got, want } = run([
      { afterMs: 0, want: true },
      { afterMs: 0, want: true },
      { afterMs: 0, want: true },
      { afterMs: 0, want: true },
      { afterMs: 0, want: true },
      { afterMs: 0, want: false }, // 6発目は空
    ]);
    expect(got).toEqual(want);
  });

  it("経過時間に比例して補充される", () => {
    const { got, want } = run([
      ...Array.from({ length: 5 }, () => ({ afterMs: 0, want: true })),
      { afterMs: 1000, want: false }, // 0.5トークンでは足りない
      { afterMs: 1000, want: true }, // 合計2秒で1トークン
      { afterMs: 0, want: false },
    ]);
    expect(got).toEqual(want);
  });

  it("放置してもトークンは容量を超えない", () => {
    const { got, want } = run([
      { afterMs: 3_600_000, want: true }, // 1時間待っても
      ...Array.from({ length: 4 }, () => ({ afterMs: 0, want: true })),
      { afterMs: 0, want: false }, // 容量の5発まで
    ]);
    expect(got).toEqual(want);
  });

  it("拒否時は retryAfterMs で次に通る時刻を案内する", () => {
    let state: BucketState | undefined;
    const now = 1_000_000;
    for (let i = 0; i < 5; i++) {
      state = take(state, now, CAP, RATE).state;
    }
    const denied = take(state, now, CAP, RATE);
    expect(denied.allowed).toBe(false);
    // 空(0トークン)から1トークン貯まるのは 1 / 0.5 = 2秒後
    expect(denied.retryAfterMs).toBe(2000);
    // 案内どおり待てば通る
    const retried = take(denied.state, now + denied.retryAfterMs, CAP, RATE);
    expect(retried.allowed).toBe(true);
  });

  it("時計が巻き戻っても壊れない(補充が負にならない)", () => {
    const first = take(undefined, 1_000_000, CAP, RATE);
    const back = take(first.state, 999_000, CAP, RATE); // 1秒巻き戻し
    expect(back.allowed).toBe(true);
    expect(back.state.tokens).toBeLessThanOrEqual(CAP);
  });

  it("state を破壊せず新しい state を返す", () => {
    const first = take(undefined, 1_000_000, CAP, RATE);
    const before = { ...first.state };
    take(first.state, 1_001_000, CAP, RATE);
    expect(first.state).toEqual(before);
  });
});
