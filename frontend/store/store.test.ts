import { describe, it, expect, vi } from "vitest";
import { createStore, combineReducers, createSelector, type Reducer, type Middleware, type Action } from "./store";

interface CounterState {
  count: number;
}
const counter: Reducer<CounterState> = (state, action) => {
  switch (action.type) {
    case "inc":
      return { count: state.count + 1 };
    case "add":
      return { count: state.count + (action.by as number) };
    default:
      return state;
  }
};

describe("createStore", () => {
  it("dispatch した action を reducer が処理して状態を更新する", () => {
    const store = createStore(counter, { count: 0 });
    expect(store.getState().count).toBe(0);
    store.dispatch({ type: "inc" });
    store.dispatch({ type: "add", by: 5 });
    expect(store.getState().count).toBe(6);
  });

  // イミュータブルの核心: dispatch しても古い状態オブジェクトは変わらない。
  it("状態はイミュータブル（古い参照は変わらない）", () => {
    const store = createStore(counter, { count: 0 });
    const before = store.getState();
    store.dispatch({ type: "inc" });
    const after = store.getState();
    expect(before).not.toBe(after); // 別オブジェクト
    expect(before.count).toBe(0); // 古い方は 0 のまま
    expect(after.count).toBe(1);
  });

  it("subscribe した listener が変更時に呼ばれ、解除で止まる", () => {
    const store = createStore(counter, { count: 0 });
    const listener = vi.fn();
    const unsub = store.subscribe(listener);
    store.dispatch({ type: "inc" });
    store.dispatch({ type: "inc" });
    expect(listener).toHaveBeenCalledTimes(2);
    unsub();
    store.dispatch({ type: "inc" });
    expect(listener).toHaveBeenCalledTimes(2); // 解除後は呼ばれない
  });

  it("未知の action では状態が変わらない", () => {
    const store = createStore(counter, { count: 3 });
    const before = store.getState();
    store.dispatch({ type: "unknown" });
    expect(store.getState()).toBe(before); // 同じ参照
  });
});

describe("combineReducers", () => {
  interface Todo {
    id: number;
    done: boolean;
  }
  const todos: Reducer<Todo[]> = (state, action) => {
    if (action.type === "addTodo") return [...state, { id: action.id as number, done: false }];
    return state;
  };
  interface AppState {
    counter: CounterState;
    todos: Todo[];
  }

  it("状態をスライスに分けて別々の reducer が担当する", () => {
    const root = combineReducers<AppState>({ counter, todos });
    const store = createStore(root, { counter: { count: 0 }, todos: [] });

    store.dispatch({ type: "inc" });
    store.dispatch({ type: "addTodo", id: 1 });

    expect(store.getState().counter.count).toBe(1);
    expect(store.getState().todos).toHaveLength(1);
  });

  it("変化のないスライスは同じ参照を保つ", () => {
    const root = combineReducers<AppState>({ counter, todos });
    const store = createStore(root, { counter: { count: 0 }, todos: [] });
    const todosBefore = store.getState().todos;
    store.dispatch({ type: "inc" }); // counter だけ変わる
    expect(store.getState().todos).toBe(todosBefore); // todos は同じ参照
  });
});

describe("createSelector", () => {
  interface State {
    items: number[];
    filter: string;
  }
  it("入力が変わるまで再計算しない（メモ化）", () => {
    const computeFn = vi.fn((items: number[]) => items.reduce((a, b) => a + b, 0));
    const total = createSelector<State, [number[]], number>([(s) => s.items], computeFn);

    const s1: State = { items: [1, 2, 3], filter: "a" };
    expect(total(s1)).toBe(6);
    // filter だけ変えて items は同じ参照 → 再計算しない。
    const s2: State = { items: s1.items, filter: "b" };
    expect(total(s2)).toBe(6);
    expect(computeFn).toHaveBeenCalledTimes(1); // items が同じなので 1 回

    // items が変わったら再計算。
    const s3: State = { items: [1, 2, 3, 4], filter: "b" };
    expect(total(s3)).toBe(10);
    expect(computeFn).toHaveBeenCalledTimes(2);
  });
});

describe("middleware", () => {
  it("dispatch を包んで前後に処理を挟める（ログ）", () => {
    const log: string[] = [];
    const logger: Middleware<CounterState> = () => (next) => (action) => {
      log.push(`before:${action.type}`);
      const r = next(action);
      log.push(`after:${action.type}`);
      return r;
    };
    const store = createStore(counter, { count: 0 }, logger);
    store.dispatch({ type: "inc" });
    expect(log).toEqual(["before:inc", "after:inc"]);
    expect(store.getState().count).toBe(1);
  });

  it("thunk 風 middleware で関数 action を処理できる", () => {
    // 関数を dispatch できるようにする middleware。
    const thunk: Middleware<CounterState> = (api) => (next) => (action) => {
      if (typeof action === "function") {
        return (action as unknown as (d: typeof api.dispatch, g: typeof api.getState) => Action)(
          api.dispatch,
          api.getState,
        );
      }
      return next(action);
    };
    const store = createStore(counter, { count: 0 }, thunk);
    // 2 回 inc する関数 action。
    const twice = (dispatch: (a: Action) => Action) => {
      dispatch({ type: "inc" });
      dispatch({ type: "inc" });
      return { type: "twiceDone" };
    };
    store.dispatch(twice as unknown as Action);
    expect(store.getState().count).toBe(2);
  });
});
