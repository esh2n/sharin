# store — 単方向データフローの状態管理

状態の変え方を一本道に縛る Redux 風の store。状態は読み取り専用で、変更は action の dispatch → 純粋 reducer → 新 state → 通知の一方向にだけ流れる。signal の暗黙のリアクティビティとは対照的に、明示的で追跡可能。

## 肝

- **単方向データフロー**: 状態を直接触らず、action を dispatch する。reducer が新しい状態を返し、購読者に通知する
- **純粋な reducer**: `(state, action) => newState`。古い状態を書き換えず新しい状態を返す。副作用なし
- **イミュータブル**: 古い状態オブジェクトは不変。だから時間旅行(過去の状態に戻す)やデバッグができる
- **combineReducers**: 状態をスライスに分け、各 reducer が自分の担当だけ更新する。変化のないスライスは同じ参照を保つ
- **セレクタ**: 状態からの派生値を、入力が変わるまでキャッシュ(メモ化)
- **middleware**: dispatch を包んで前後に処理を挟む(ログ、非同期の thunk など)

## 効果の固定(テスト)

- 状態はイミュータブル(dispatch しても古い参照は変わらない)
- 未知の action では状態が変わらない(同じ参照)
- combineReducers で変化のないスライスは同じ参照を保つ
- createSelector が入力の変わるまで再計算しない(メモ化)
- middleware でログ・thunk 風の関数 action を処理

## 使い方

```ts
const store = createStore(reducer, { count: 0 }, logger);
const unsub = store.subscribe(() => render(store.getState()));
store.dispatch({ type: "inc" }); // 状態変更は必ずこの一本道
const total = createSelector([(s) => s.items], (items) => items.length);
```

## 簡略化したこと

- **非同期は thunk のみ**: saga や observable ベースの非同期は扱わない
- **不変性は約束**: reducer が誤って mutate しても防げない(実物は Immer や凍結で守る)
- **DevTools 連携なし**: 時間旅行 UI やアクション記録は概念のみ
- **型は簡略**: action は文字列 type + 任意フィールド。実物は判別可能ユニオン

## 章

教科書: [状態管理(store)](https://sharin-2a1.pages.dev/parts/store)

実行: `pnpm test`(frontend/ で vitest)
