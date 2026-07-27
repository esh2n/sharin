# reactivity — signal ベースのきめ細かなリアクティビティ

状態そのものが「誰が読んだか」を覚え、変わったときにその読み手だけを再実行する。仮想DOMの「全体を作り直して差分」とは対照的な、木を比べないリアクティビティ。Solid / Vue / Preact Signals の心臓部。

## 肝

- **signal**: 値 + 購読者集合。読むと実行中のエフェクトを購読者に登録する（依存追跡）
- **effect**: signal を読む関数。読んだ signal が変わると自動で再実行される。dispose で停止
- **computed**: 依存が変わるまで再計算しない派生値（遅延評価 + キャッシュ）。読まれなければ計算もしない
- **きめ細かさ**: 読んだ signal だけを購読する。無関係な signal の変更では再実行されない
- **動的な依存**: 再実行のたびに購読を張り直すので、条件で読む signal が変わると購読も切り替わる
- **仮想DOMとの対比**: vdom は木を diff する。signal は依存グラフをたどって変わった箇所だけ更新する

## 効果の固定(テスト)

- 読んだ signal だけを購読（読んでいない signal の変更では再実行しない）
- 動的な依存（読まなくなった signal からは購読が外れる）
- computed のキャッシュ（依存が変わるまで再計算しない）と遅延評価（読まれるまで計算しない）

## 使い方

```ts
const count = signal(0);
const doubled = computed(() => count.get() * 2);
const stop = effect(() => console.log(doubled.get())); // 0
count.set(5); // effect が再実行され 10
stop();       // 以後は再実行されない
```

## 簡略化したこと

- **バッチ処理なし**: 複数の set をまとめて 1 回の再実行にする batch は省略
- **同期実行**: set で即座に effect が走る。実物はマイクロタスクでまとめることが多い
- **DOM 連携なし**: signal を DOM 更新に繋ぐ部分は扱わない（vdom / テンプレートの役目）
- **循環依存の検出なし**: 依存が輪を作ると無限ループしうる

## 章

教科書: [リアクティビティ(signal)](https://sharin-2a1.pages.dev/parts/reactivity)

実行: `pnpm test`(frontend/ で vitest)
