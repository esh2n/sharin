// きめ細かなリアクティビティ（signal ベース）を最小構成でフルスクラッチする。
//
// 仮想DOMは「状態が変わったら全体を作り直し、差分を当てる」やり方だった。
// リアクティビティは逆に、状態そのものが「誰が自分を読んだか」を覚えておき、
// 変わったときにその読み手だけを再実行する。木を丸ごと比べる必要がない。
// Solid / Vue / Preact Signals の心臓部がこれだ。
//
// 肝は3つ:
//   1. signal = 値 + 購読者の集合。読むと今実行中のエフェクトを購読者に登録する
//   2. effect = signal を読む関数。読んだ signal が変わると自動で再実行される
//   3. computed = 派生値。依存が変わるまで再計算せずキャッシュする（遅延評価）

// #region signal{ts}

// Effect は再実行できる仕事の単位。deps は自分が購読している購読者集合の逆参照
// （再実行前に古い購読を解除するため）。
interface Effect {
  run: () => void;
  deps: Set<Set<Effect>>;
}

// activeEffect は「今まさに実行中」のエフェクト。signal はこれを読み取って
// 「誰が自分を読んだか」を知る（依存追跡）。
let activeEffect: Effect | null = null;

// track は signal / computed が読まれたときに、実行中のエフェクトを購読者に登録する。
function track(subs: Set<Effect>): void {
  if (activeEffect) {
    subs.add(activeEffect);
    activeEffect.deps.add(subs); // 逆参照も張る
  }
}

// Signal は 1 つの値と、その値を読んだエフェクトの集合を持つ。
export interface Signal<T> {
  get(): T;
  set(v: T): void;
}

// signal は読み書きできるリアクティブな値を作る。
export function signal<T>(initial: T): Signal<T> {
  let value = initial;
  const subs = new Set<Effect>();
  return {
    get() {
      track(subs); // 読んだエフェクトを購読者に登録
      return value;
    },
    set(v: T) {
      if (Object.is(v, value)) return; // 変化なしなら何もしない
      value = v;
      // 反復中の購読変更に備えてコピーしてから走らせる。
      for (const e of [...subs]) e.run();
    },
  };
}

// #endregion signal{ts}

// #region effect{ts}

// cleanup はエフェクトの購読をすべて解除する（再実行前に呼ぶ）。
// これにより、今回読まなかった signal からは購読が外れる（動的な依存）。
function cleanup(e: Effect): void {
  for (const dep of e.deps) dep.delete(e);
  e.deps.clear();
}

// effect は fn を即座に実行し、fn が読んだ signal が変わるたびに再実行する。
// 戻り値を呼ぶと購読を解除して以後再実行されなくなる（dispose）。
export function effect(fn: () => void): () => void {
  const e: Effect = {
    deps: new Set(),
    run() {
      cleanup(e); // 前回の依存を捨てる
      const prev = activeEffect;
      activeEffect = e; // 実行中はこのエフェクトを active にする
      try {
        fn(); // fn 内で読まれた signal が track され購読が張られる
      } finally {
        activeEffect = prev;
      }
    },
  };
  e.run();
  return () => cleanup(e);
}

// #endregion effect{ts}

// #region computed{ts}

// Computed は依存が変わるまで再計算しない派生値（遅延 + キャッシュ）。
export interface Computed<T> {
  get(): T;
}

// computed は fn の結果をキャッシュし、依存が変わったら「汚れた」印だけ付ける。
// 次に get されたときに初めて再計算する（遅延評価）。読まれなければ計算もしない。
export function computed<T>(fn: () => T): Computed<T> {
  const subs = new Set<Effect>(); // この computed を読んだエフェクト
  let value: T;
  let stale = true;

  // 依存が変わると呼ばれる内部エフェクト。値は捨てず、汚れ印を付けて
  // 自分の購読者に伝播するだけ（連鎖する computed のため）。
  const runner: Effect = {
    deps: new Set(),
    run() {
      if (!stale) {
        stale = true;
        for (const e of [...subs]) e.run();
      }
    },
  };

  return {
    get() {
      if (stale) {
        cleanup(runner);
        const prev = activeEffect;
        activeEffect = runner; // 再計算中に読む signal は runner が購読
        try {
          value = fn();
        } finally {
          activeEffect = prev;
        }
        stale = false;
      }
      track(subs); // この computed を読んだエフェクトを購読者に
      return value;
    },
  };
}

// #endregion computed{ts}
