# bundler — 依存解決とバンドル

入口(entry)から import を辿って依存グラフを作り、依存が先に来るよう並べ替え、循環を検出し、使われない export を落とす。バンドラの中核である依存解決を最小構成で実装する。

## 肝

- **依存グラフ**: entry から import を辿り、到達できるモジュールだけを集める。どこからも import されないモジュールはバンドルに含まれない
- **トポロジカル順序**: 依存を先に、それを使う側を後に並べる。DFS の帰りがけ順がこの順になる
- **循環依存の検出**: DFS で訪問中(gray)のノードに戻れば循環。経路を返す
- **tree-shaking**: 実際に import される export だけ残し、どこからも使われない export を落とす
- **entry は全 export を残す**: 入口はアプリの実行対象なので、その export は落とさない

## 効果の固定(テスト)

- `collectReachable`: 孤立モジュールをバンドルに含めない
- `topoOrder`: 依存(math)が使う側(util, entry)より前に並ぶ
- 循環検出: a→b→c→a を CycleError で経路つきで検出
- tree-shaking: 誰も使わない export(mul)を落とす

## 使い方

```ts
const reachable = collectReachable("entry", registry); // バンドル対象
const order = topoOrder("entry", registry);            // 連結順(依存が先)
const cycle = hasCycle("entry", registry);             // 循環の経路 or null
const kept = treeShake("entry", registry);             // 各モジュールの生き残る export
```

## 簡略化したこと

- **パースなし**: 実物はソースを AST に解析して import/export を抽出。ここは宣言済みの依存グラフを受け取る
- **副作用の考慮なし**: 実物は副作用のある import は tree-shake できない。ここは純粋前提
- **コード生成なし**: 順序と生存 export を出すまで。実際の連結・変換・minify は扱わない
- **動的 import なし**: `import()` によるコード分割は扱わない

## 章

教科書: [バンドラ(依存解決)](https://sharin-2a1.pages.dev/parts/bundler)

実行: `pnpm test`(frontend/ で vitest)
