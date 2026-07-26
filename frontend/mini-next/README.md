# mini-next — SSR + ルーティング + ハイドレーション

サーバで HTML を組み、URL でページを選び、届いた静的HTMLにイベントを後付けして動かす——
Next.js のようなフレームワークの核だけを、[vdom](../vdom) の上に最小構成で作る。

教科書の章: [mini-next](https://sharin-2a1.pages.dev/parts/mini-next)

## これは何か

[vdom](../vdom) は「仮想DOM木 → 実DOM」だった。mini-next はその木を**別の3つの用途**に回す:

- **SSR** (`renderToString`): 木を実DOMでなく **HTML文字列**にする。サーバで組んで返せば初回表示が速く、クローラも読める
- **ルーティング** (`createRouter`): URL のパスを**ページコンポーネント**に対応づける。`/posts/:id` の動的セグメントも取り出す
- **ハイドレーション** (`hydrate`): サーバHTMLを**作り直さず**、同じ木からイベントリスナだけを既存DOMに後付けする

`app.ts` がこの3つを繋ぐ: サーバは `renderRoute(router, url)` で HTML を返し、クライアントは
`hydrateRoute(router, url, container)` でその上にイベントを付けて対話可能にする。

## 肝は3つ

1. **同じ木を3通りに使う**: 1つの仮想DOM木を、実DOM化(vdom)・文字列化(SSR)・イベント付け(hydrate)に回す。コンポーネントは描き先を知らなくてよい
2. **ハイドレーション = 作り直さない**: サーバ描画済みの実DOMは既にある。捨てて mount し直すのは無駄でちらつく。既存ノードを使い回し、文字列に乗らなかったイベントだけを付ける
3. **SSR は Node で動く**: `renderToString` は文字列を組むだけで実DOMを触らない。だからブラウザの無いサーバでも走る（テストは happy-dom だが、SSR 自体に DOM は不要）

## 設計メモ

- **エスケープ**: テキストは `< > &`、属性値は `" &` を実体参照にする。怠るとタグ注入(XSS)になる最初の一歩
- **void 要素**: `br`/`img`/`input` 等は閉じタグを出さない
- **イベントは SSR に乗らない**: 関数は文字列化できないので出力せず、hydrate で付け直す。この「SSR で欠ける分を hydrate が埋める」対称性が肝
- **ルータは登録順で先頭一致**: 静的 route を動的より先に置けば優先できる素朴な方式

## 簡略化したこと

- **状態管理・再描画なし**: hydrate 後の状態変化→再描画のループは無し（vdom の diff を繋げば作れる）。ここは「静的HTML→対話可能」への接続まで
- **ハイドレーション不一致の検出なし**: サーバとクライアントで木がズレたときの警告や修復は無し（React の hydration mismatch 相当）
- **ネストルート・レイアウト・データ取得なし**: `getServerSideProps` 的なデータ取得、ネストレイアウト、コード分割は無し
- **ワイルドカード/オプショナルセグメントなし**: `:id` の単純捕獲のみ。`[...slug]` は無し
- **ストリーミングSSR・部分ハイドレーションなし**: 一括で文字列化・一括で hydrate

## 動かす

```bash
cd frontend
pnpm test        # vitest（happy-dom）
pnpm cov         # カバレッジ
pnpm typecheck   # tsc --noEmit（strict）
```

このMacでは `mise exec node@22.20.0 -- npx pnpm@9.15.0 <cmd>`。
