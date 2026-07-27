// 単方向データフローの状態管理（Redux 風）を最小構成でフルスクラッチする。
//
// signal のリアクティビティは「状態が購読者を覚える」暗黙の仕組みだった。
// store は逆に、状態の変え方を明示的に一本道へ縛る。状態は読み取り専用で、
// 変更は必ず action を dispatch する。action は純粋な reducer に渡り、
// reducer は古い状態を書き換えず新しい状態を返す。変更のたびに購読者へ通知する。
// この一方向の流れが、状態変化を追跡可能で予測可能にする。
//
// 肝は3つ:
//   1. 単方向: 状態を直接触らず action → reducer → 新state → 通知の一本道
//   2. 純粋な reducer + イミュータブル: 古い状態は不変。時間旅行やデバッグができる
//   3. セレクタ: 状態からの派生値を、入力が変わるまでキャッシュする

// #region store{ts}

export interface Action {
  type: string;
  [key: string]: unknown;
}

export type Reducer<S> = (state: S, action: Action) => S;
export type Listener = () => void;
export type Dispatch = (action: Action) => Action;

export interface Store<S> {
  getState(): S;
  dispatch: Dispatch;
  subscribe(listener: Listener): () => void;
}

// MiddlewareAPI は middleware が使える最小の store インターフェース。
export interface MiddlewareAPI<S> {
  getState(): S;
  dispatch: Dispatch;
}

// Middleware は dispatch を包んで前後に処理を挟む（ログ、非同期など）。
export type Middleware<S> = (api: MiddlewareAPI<S>) => (next: Dispatch) => Dispatch;

// createStore は reducer と初期状態から store を作る。middleware で dispatch を拡張できる。
export function createStore<S>(reducer: Reducer<S>, preloaded: S, ...middlewares: Middleware<S>[]): Store<S> {
  let state = preloaded;
  const listeners = new Set<Listener>();

  // 素の dispatch: reducer で新しい状態を作り、購読者へ通知する。
  const baseDispatch: Dispatch = (action) => {
    state = reducer(state, action); // 直接書き換えず、返り値で置き換える
    for (const l of [...listeners]) l();
    return action;
  };

  // middleware を右から巻いて dispatch を作る。
  let dispatch: Dispatch = baseDispatch;
  if (middlewares.length > 0) {
    const api: MiddlewareAPI<S> = { getState: () => state, dispatch: (a) => dispatch(a) };
    const chain = middlewares.map((m) => m(api));
    dispatch = chain.reduceRight((next, mw) => mw(next), baseDispatch);
  }

  return {
    getState: () => state,
    dispatch,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener); // 購読解除
    },
  };
}

// #endregion store{ts}

// #region combine{ts}

// combineReducers は複数の reducer を、状態のスライスごとに束ねる。
// 各 reducer は自分の担当スライスだけを見て更新する。
export function combineReducers<S>(reducers: {
  [K in keyof S]: Reducer<S[K]>;
}): Reducer<S> {
  const keys = Object.keys(reducers) as (keyof S)[];
  return (state, action) => {
    let changed = false;
    const next = {} as S;
    for (const key of keys) {
      const prevSlice = state[key];
      const nextSlice = reducers[key](prevSlice, action);
      next[key] = nextSlice;
      if (nextSlice !== prevSlice) changed = true;
    }
    return changed ? next : state; // 何も変わらなければ同じ参照を返す
  };
}

// #endregion combine{ts}

// #region selector{ts}

// createSelector は状態からの派生値を、入力が変わるまでキャッシュする（メモ化）。
// inputs で状態から値を取り出し、それらが前回と同じなら result を再計算しない。
export function createSelector<S, Inputs extends unknown[], R>(
  inputs: { [K in keyof Inputs]: (state: S) => Inputs[K] },
  result: (...args: Inputs) => R,
): (state: S) => R {
  let lastArgs: Inputs | null = null;
  let lastResult: R;
  return (state) => {
    const args = inputs.map((fn) => fn(state)) as Inputs;
    if (lastArgs !== null && args.length === lastArgs.length && args.every((a, i) => Object.is(a, lastArgs![i]))) {
      return lastResult; // 入力が同じ = 再計算しない
    }
    lastArgs = args;
    lastResult = result(...args);
    return lastResult;
  };
}

// #endregion selector{ts}
