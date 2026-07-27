import { describe, it, expect } from "vitest";
import { signal, effect, computed } from "./reactivity";

describe("signal と effect", () => {
  it("signal を変えると、それを読んだ effect が再実行される", () => {
    const count = signal(0);
    const seen: number[] = [];
    effect(() => seen.push(count.get()));

    expect(seen).toEqual([0]); // 初回に即実行
    count.set(1);
    count.set(2);
    expect(seen).toEqual([0, 1, 2]);
  });

  it("同じ値をセットしても再実行しない", () => {
    const s = signal(5);
    let runs = 0;
    effect(() => {
      s.get();
      runs++;
    });
    expect(runs).toBe(1);
    s.set(5); // 変化なし
    expect(runs).toBe(1);
  });

  // きめ細かさの核心: 読んでいない signal の変更では再実行されない。
  it("読んだ signal だけを購読する（きめ細かさ）", () => {
    const a = signal("a");
    const b = signal("b");
    let runs = 0;
    effect(() => {
      a.get(); // a だけ読む
      runs++;
    });
    expect(runs).toBe(1);
    b.set("b2"); // 読んでいない b を変えても
    expect(runs).toBe(1); // 再実行されない
    a.set("a2");
    expect(runs).toBe(2);
  });

  // 動的な依存: 条件で読む signal が変わると購読も切り替わる。
  it("読まなくなった signal からは購読が外れる", () => {
    const use = signal(true);
    const a = signal("a");
    const b = signal("b");
    let runs = 0;
    effect(() => {
      use.get() ? a.get() : b.get();
      runs++;
    });
    expect(runs).toBe(1); // use=true なので a を購読

    use.set(false); // b を購読に切替（a は外れる）
    expect(runs).toBe(2);

    a.set("a2"); // もう a は読んでいない
    expect(runs).toBe(2); // 再実行されない

    b.set("b2"); // 今は b を読んでいる
    expect(runs).toBe(3);
  });

  it("dispose すると再実行が止まる", () => {
    const s = signal(0);
    let runs = 0;
    const stop = effect(() => {
      s.get();
      runs++;
    });
    s.set(1);
    expect(runs).toBe(2);
    stop();
    s.set(2);
    expect(runs).toBe(2); // 止まっている
  });
});

describe("computed", () => {
  it("依存から派生値を計算する", () => {
    const w = signal(2);
    const h = signal(3);
    const area = computed(() => w.get() * h.get());
    expect(area.get()).toBe(6);
    w.set(4);
    expect(area.get()).toBe(12);
  });

  // キャッシュの核心: 依存が変わるまで再計算しない。
  it("依存が変わるまで再計算せずキャッシュする", () => {
    const s = signal(10);
    let computes = 0;
    const double = computed(() => {
      computes++;
      return s.get() * 2;
    });

    expect(double.get()).toBe(20);
    expect(double.get()).toBe(20);
    expect(computes).toBe(1); // 2 回読んでも計算は 1 回（キャッシュ）

    s.set(11); // 依存が変わった
    expect(double.get()).toBe(22);
    expect(computes).toBe(2); // ここで初めて再計算
  });

  // 遅延評価: 読まれなければ計算しない。
  it("読まれるまで計算しない（遅延評価）", () => {
    const s = signal(1);
    let computes = 0;
    const c = computed(() => {
      computes++;
      return s.get();
    });
    expect(computes).toBe(0); // 作っただけでは計算しない
    c.get();
    expect(computes).toBe(1);
  });

  it("computed を読む effect は、依存 signal の変化で再実行される", () => {
    const s = signal(1);
    const doubled = computed(() => s.get() * 2);
    const seen: number[] = [];
    effect(() => seen.push(doubled.get()));
    expect(seen).toEqual([2]);
    s.set(5);
    expect(seen).toEqual([2, 10]);
  });

  it("computed を連鎖できる", () => {
    const s = signal(1);
    const a = computed(() => s.get() + 1); // 2
    const b = computed(() => a.get() * 10); // 20
    expect(b.get()).toBe(20);
    s.set(2);
    expect(b.get()).toBe(30); // s→a→b と伝播
  });
});
