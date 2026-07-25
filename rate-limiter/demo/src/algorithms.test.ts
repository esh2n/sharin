import { describe, expect, it } from "vitest";
import { ALGOS, type Algo, type AlgoState, LIMIT, WINDOW_MS, apply } from "./algorithms";

// (経過ms, 期待) のステップ列を流して allowed の列を突き合わせるヘルパー。
function run(algo: Algo, steps: Array<{ afterMs: number; want: boolean }>) {
  let state: AlgoState | undefined;
  // fixed window の境界計算が決定的になるよう WINDOW_MS 境界に揃えて開始する。
  let now = WINDOW_MS * 100;
  const got: boolean[] = [];
  for (const s of steps) {
    now += s.afterMs;
    const r = apply(algo, state, now);
    state = r.state;
    got.push(r.allowed);
  }
  return { got, want: steps.map((s) => s.want) };
}

const burst = (n: number, want: boolean) => Array.from({ length: n }, () => ({ afterMs: 0, want }));

describe("全方式共通", () => {
  for (const algo of ALGOS) {
    it(`${algo}: 連打はちょうど ${LIMIT} 発まで通る`, () => {
      const { got, want } = run(algo, [...burst(LIMIT, true), ...burst(2, false)]);
      expect(got).toEqual(want);
    });
  }
});

describe("token-bucket / leaky-bucket", () => {
  for (const algo of ["token-bucket", "leaky-bucket"] as const) {
    it(`${algo}: 0.5個/秒で回復する(2秒で1発)`, () => {
      const { got, want } = run(algo, [
        ...burst(LIMIT, true),
        { afterMs: 1000, want: false },
        { afterMs: 1000, want: true },
        { afterMs: 0, want: false },
      ]);
      expect(got).toEqual(want);
    });
  }
});

describe("fixed-window", () => {
  it("境界バースト: 窓が切り替わった直後にまた limit 発通る", () => {
    const { got, want } = run("fixed-window", [
      { afterMs: WINDOW_MS - 500, want: true }, // 窓の終わり際に
      ...burst(LIMIT - 1, true),
      { afterMs: 0, want: false },
      { afterMs: 1000, want: true }, // 次の窓に入った瞬間
      ...burst(LIMIT - 1, true),
      { afterMs: 0, want: false },
    ]);
    expect(got).toEqual(want);
  });

  it("拒否時の retryAfterMs は窓の残り時間", () => {
    let state: AlgoState | undefined;
    const start = WINDOW_MS * 100;
    for (let i = 0; i < LIMIT; i++) {
      state = apply("fixed-window", state, start).state;
    }
    const denied = apply("fixed-window", state, start + 4000);
    expect(denied.allowed).toBe(false);
    expect(denied.retryAfterMs).toBe(WINDOW_MS - 4000);
  });
});

describe("sliding-window-log", () => {
  it("境界バーストを許さない(fixed window と対比)", () => {
    const { got, want } = run("sliding-window-log", [
      { afterMs: WINDOW_MS - 500, want: true },
      ...burst(LIMIT - 1, true),
      { afterMs: 1000, want: false }, // 窓が滑るので直近 WINDOW_MS に LIMIT 件残っている
    ]);
    expect(got).toEqual(want);
  });

  it("古い記録が窓から抜けた分だけ1件ずつ回復する", () => {
    const { got, want } = run("sliding-window-log", [
      { afterMs: 0, want: true },
      { afterMs: 1000, want: true },
      ...burst(LIMIT - 2, true),
      { afterMs: 0, want: false },
      { afterMs: WINDOW_MS - 1000, want: true }, // 1件目だけが窓外
      { afterMs: 0, want: false }, // 2件目はまだ窓内
      { afterMs: 1000, want: true },
    ]);
    expect(got).toEqual(want);
  });
});

describe("共通の性質", () => {
  for (const algo of ALGOS) {
    it(`${algo}: 時計が巻き戻っても壊れない`, () => {
      const t0 = WINDOW_MS * 100;
      const first = apply(algo, undefined, t0);
      const back = apply(algo, first.state, t0 - 5000);
      expect(typeof back.allowed).toBe("boolean");
      expect(back.remaining).toBeGreaterThanOrEqual(0);
    });

    it(`${algo}: 入力の state を破壊しない`, () => {
      const t0 = WINDOW_MS * 100;
      const first = apply(algo, undefined, t0);
      const snapshot = JSON.parse(JSON.stringify(first.state));
      apply(algo, first.state, t0 + 1000);
      expect(first.state).toEqual(snapshot);
    });
  }
});
