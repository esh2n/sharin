<script setup>
import StoreDemo from '../components/StoreDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 状態管理(store)

> 実装: [`frontend/store/`](https://github.com/esh2n/sharin/tree/main/frontend/store) / 実行: `pnpm test`(frontend/)

<Summary>
アプリが育つと状態がどこからでも変わり、追えなくなる。storeはこれを一本道に縛る。状態は読み取り専用で、変更は必ずactionをdispatchする。actionは純粋なreducerに渡り、古い状態を書き換えず新しい状態を返して購読者へ通知する。この一方向の流れが状態変化を追跡可能にする。中核とcombineReducers・セレクタ・middlewareを実装する。
</Summary>

## この章で作るもの

[リアクティビティ(signal)](/parts/reactivity)は、状態が変わると購読者が自動で反応する暗黙の仕組みだった。手軽な反面、規模が大きくなると「この状態は、いつ、どこから変えられたのか」が追いにくくなる。あちこちのコンポーネントが直接状態を書き換えると、バグが起きたとき原因の特定が難しい。store は逆の哲学を採る。状態の変え方を、たった一本の道に縛る。

その道が単方向データフローだ。状態は読み取り専用で、誰も直接書き換えられない。変えたいときは action(「カウントを増やす」という意図のオブジェクト)を dispatch する。action は純粋な reducer に渡り、reducer は今の状態と action から新しい状態を計算して返す。古い状態は一切変えない。新しい状態ができたら、購読しているすべての場所に通知する。すべての状態変化がこの一本道を通るので、「何が状態を変えたか」は必ず action として記録に残る。Redux が広めたこの構えを、この章では最小構成で実装する。

<FigureBox caption="単方向データフロー。状態を直接触らず、action を dispatch。reducer が新しい状態を返し、購読者に通知する。流れは常に一方向">

```
   dispatch(action)
        │
        ▼
   ┌─ reducer ─┐   (state, action) → 新state(古いstateは不変)
   │  純粋関数  │
   └─────┬─────┘
         ▼
   新しい state ──通知──▶ 購読者(UI を再描画)
         ▲
         └── 次の dispatch へ(一方向の輪)
```

</FigureBox>

肝は3つ:

1. **単方向データフロー**: 状態を直接触らず、action → reducer → 新state → 通知の一本道
2. **純粋な reducer とイミュータブル**: 古い状態は不変。だから予測可能で、時間旅行やデバッグができる
3. **分割とキャッシュ**: combineReducers で状態を分け、セレクタで派生値をメモ化する

## ① store の中核: dispatch・reducer・subscribe

まず store の心臓を作る。状態を持ち、action を dispatch すると reducer で新しい状態にし、購読者へ通知する:

<<< ../../frontend/store/store.ts#store{ts}

`baseDispatch` が一本道そのものだ。`state = reducer(state, action)` で、reducer の返り値に状態を置き換える。ここで reducer は古い状態を書き換えず、新しいオブジェクトを返すのが約束だ。だから dispatch の前後で、古い状態オブジェクトは変わらない。テストで、dispatch しても前の状態の参照と値が変わらないこと(イミュータブル)、未知の action では状態が同じ参照のままなことを固定した。この不変性が効くのは、UI が「状態が変わったか」を参照の比較(`before !== after`)だけで判定できるからだ。中身を深く比べる必要がない。middleware は、この dispatch を包んで前後に処理を挟む拡張点だ(後述)。

## ② combineReducers: 状態を分割する

アプリの状態は 1 つの塊ではなく、複数の関心事(カウンタ、todoリスト、ユーザ情報…)の集まりだ。1 つの巨大な reducer にすべてを詰めると手に負えなくなる。combineReducers は、状態をスライスに分け、各スライスを専用の reducer に担当させる:

<<< ../../frontend/store/store.ts#combine{ts}

各 reducer は自分の担当スライスだけを見て、そのスライスの新しい値を返す。全体の reducer は、それらをまとめて新しい状態オブジェクトを組む。ここで大事なのが、変化のないスライスは同じ参照を保つことだ。カウンタだけを変える action では、todos スライスの reducer は同じ配列をそのまま返すので、todos の参照は変わらない。テストで、`inc` action の後も todos が同じ参照であることを固定した。これにより、UI は「todos は変わっていない」を参照比較だけで判定でき、todos に依存する部分の再描画を丸ごとスキップできる。

## ③ セレクタと middleware

状態から派生した値(「完了した todo の数」「合計金額」)を、UI は頻繁に読む。毎回計算し直すのは無駄だ。セレクタは、入力(状態の一部)が変わるまで結果をキャッシュする:

<<< ../../frontend/store/store.ts#selector{ts}

`createSelector` は、`inputs` で状態から値を取り出し、それらが前回と同じ参照なら `result` を再計算しない。combineReducers が「変化のないスライスは同じ参照」を保証してくれるので、これがうまく噛み合う。todos が変わらなければ todos の参照も変わらず、todos から派生するセレクタは再計算をスキップする。テストで、入力の配列が同じ参照の間は計算関数が一度しか呼ばれないことを固定した。

middleware は dispatch を包む拡張点だ。`() => (next) => (action) => {...}` という三段の関数で、action を `next` に渡す前後に好きな処理を挟める。ログを取る、action を記録する、あるいは関数を dispatch できるようにして非同期処理を扱う(thunk)。テストで、ログ middleware が action の前後を記録すること、thunk 風 middleware が関数 action を処理して複数の dispatch をまとめられることを固定した。dispatch という一点を包むだけで、横断的な関心事を差し込める。

### 動かす

下のデモは、action を dispatch して状態が一本道で変わる様子と、middleware が dispatch を包んでログを取る様子を見る。状態のイミュータブルな更新、購読者への通知、セレクタのキャッシュも確かめられる。

<StoreDemo />

## 設計の観点

- **単方向は追跡可能性のため**: すべての変更が action を通るので、「何が状態を変えたか」が記録に残る。時間旅行デバッグ(action を巻き戻す)が可能になる
- **reducer は純粋に保つ**: API 呼び出しや乱数など副作用を reducer に入れない。純粋だからこそ、同じ入力で同じ結果になり、テストとリプレイができる。副作用は middleware や外側へ
- **イミュータブルが参照比較を可能にする**: 状態を書き換えず新オブジェクトを返すことで、UI は参照の等値だけで変更を検出できる。深い比較が要らず速い
- **signal との使い分け**: signal は局所的できめ細かい状態に手軽。store は大域的で、変更履歴の追跡や予測可能性が要る状態に向く。両者は排他でなく、併用もされる
- **ボイラープレートの代償**: action・reducer・型の定義は冗長になりがち。小さなアプリには過剰で、その反省から Zustand のような軽量な store も生まれた

## 対照と実例

| | 直接変更 | signal | store(Redux) |
|---|---|---|---|
| 変更の起点 | どこからでも | signal.set | dispatch のみ |
| 追跡可能性 | 低い | 中 | 高い(action 記録) |
| 粒度 | — | きめ細かい | 中央集権 |
| 派生値 | 手動 | computed | セレクタ |
| 向く規模 | 小 | 中 | 大・複雑 |

裏どり:

- **Redux**: 単方向データフロー・純粋 reducer・middleware の原型。この章の API はこれに近い
- **Flux (Facebook)**: Redux の前身。単方向データフローの考え方を広めた
- **Zustand / Jotai**: Redux のボイラープレートを減らした軽量 store。用途で使い分ける
- **Redux DevTools**: action を記録し、状態を巻き戻せる時間旅行デバッガ。イミュータブルだから実現できる

## 簡略化したこと

- **非同期は thunk のみ**: saga や observable ベースの非同期は扱わない
- **不変性は約束**: reducer が誤って mutate しても防げない。実物は Immer や凍結で守る
- **DevTools 連携なし**: 時間旅行 UI やアクション記録は概念のみ
- **型は簡略**: action は文字列 type + 任意フィールド。実物は判別可能ユニオンで厳密に

## 参考資料

- [Redux: Core Concepts](https://redux.js.org/tutorials/fundamentals/part-2-concepts-data-flow) — 単方向データフローと reducer
- [Redux: Middleware](https://redux.js.org/understanding/history-and-design/middleware) — dispatch を包む設計
- [Reselect](https://github.com/reduxjs/reselect) — createSelector のメモ化の元
- 実装: [frontend/store](https://github.com/esh2n/sharin/tree/main/frontend/store)
