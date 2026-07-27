<script setup>
import MiniNextDemo from '../components/MiniNextDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# mini-next(SSR + ルーティング + ハイドレーション)

<Summary>
サーバで HTML を組み、URL でページを選び、届いた静的 HTML にイベントを後付けして動かす。Next.js のようなフレームワークの核だけを、仮想DOM の上に作る。鍵は 1 つの仮想DOM 木を実 DOM 化・文字列化(SSR)・イベント付け(hydrate)の 3 通りに使うことだ。ハイドレーションは作り直さず、サーバ描画済みの実 DOM を使い回して、文字列に載らなかったイベントだけを付ける。SSR は文字列を組むだけなので、ブラウザの無いサーバでも走る。
</Summary>

## この章で作るもの

[仮想DOM](./vdom)は「仮想DOM木 → 実DOM」を作った。だが実際のフレームワークは、同じ木を**もっと別の使い方**もする。最初の画面を**サーバで HTML にして返し**(速い・SEO に効く)、その静的HTMLをクライアントで**対話可能にする**。この2つを繋ぐのが SSR とハイドレーションで、どのページを描くかを決めるのがルーティング。

作るのは3つ:

- **SSR** (`renderToString`): 仮想DOM木を実DOMでなく **HTML文字列**にする
- **ルーティング** (`createRouter`): URL のパスを**ページコンポーネント**に対応づける
- **ハイドレーション** (`hydrate`): サーバHTMLを**作り直さず**、イベントリスナだけを既存DOMに後付けする

肝は3つ:

1. **同じ木を3通りに使う**: コンポーネントが返す仮想DOM木を、実DOM化・文字列化・イベント付けの3方向に回す。コンポーネントは「どこに描かれるか」を知らなくてよい
2. **ハイドレーション = 作り直さない**: サーバ描画済みの実DOMを捨てず、文字列に載らなかったイベントだけを付ける
3. **SSR は Node で動く**: `renderToString` は文字列連結だけ。実DOMが無いサーバでも走る

## 全体像: 1つの木、3つの行き先

<FigureBox caption="コンポーネントが返す1つの仮想DOM木の3つの行き先。サーバでは文字列化(SSR)して返す。ブラウザはその文字列を静的DOMとして描く。そこへ同じ木からイベントだけを後付け(hydrate)して対話可能にする。mount(vdom章)は『DOMがまだ無い』場面用で、hydrate は『DOMが既にある』場面用">

```
              コンポーネント → 仮想DOM木
                     │
        ┌────────────┼─────────────────┐
        ▼            ▼                  ▼
   renderToString   mount(vdom)      hydrate
   (HTML文字列)     (実DOMを新規作成)  (既存DOMにイベント付与)
        │                               ▲
        ▼                               │
   サーバが返す ──▶ ブラウザが静的描画 ──┘
                    (この時点では“動かない”)
```

</FigureBox>

`mount`(前章)と `hydrate` の使い分けが要。DOM がまだ無いなら `mount` で作る。DOM が既にある(サーバが描いた)なら、作り直さず `hydrate` でイベントだけ足す。

## SSR: 木を HTML文字列にする

サーバには実DOM(`document`)が無い。だから木を**文字列**として組み立てる。実DOMを一切触らないので Node でそのまま動く:

<<< ../../frontend/mini-next/ssr.ts#render{ts}

要点は2つ。1 つ目はイベント(on\*)を出力しないこと。関数は文字列にできないので、ここで欠ける。この欠けをクライアントの hydrate が埋める、という対称性が後で効く。もう一つはエスケープだ:

<<< ../../frontend/mini-next/ssr.ts#escape{ts}

テキストに `<script>` が混ざったとき、エスケープしないとそれがタグとして解釈される(XSS)。SSR は「文字列を組む」ので、この危険と常に隣り合わせ。テキストは `< > &`、属性値は `" &` を実体参照にするのが最初の防御。

## ルーティング: URL → ページ

どのページを描くかは URL で決まる。`/posts/42` の `42` のような**動的セグメント**を取り出せると、記事ページを1つのテンプレートで書ける:

<<< ../../frontend/mini-next/router.ts#router{ts}

パスをセグメント配列(`/posts/42` → `["posts","42"]`)に割り、route のパターンと**位置ごとに**突き合わせる。`:id` は任意の1セグメントを捕まえて `params` に入れる。Next.js の `pages/posts/[id].tsx` がファイル名でやっていることを、明示的な配列でやっているだけ。

## ハイドレーション: 作り直さず、イベントだけ足す

ここが SSR とセットの肝。サーバが返した HTML はブラウザで**実DOMとして既に描かれている**。でもイベントは載っていないので、ボタンを押しても動かない。ここで木を `mount` し直したら、同じDOMをもう一度作ることになり無駄で、一瞬ちらつく。

代わりに、既存のDOMをたどりながら、木の on\* をリスナとして付けていく:

<<< ../../frontend/mini-next/hydrate.ts#hydrate{ts}

新しいノードは1つも作らない。サーバHTMLで欠けていた「イベント」だけを、位置を頼りに後付けする。これがハイドレーション。SSR で欠けた分を、ちょうど補完する。

### 動かす

下のデモで、URL を選ぶとルータがページを解決し、SSR が HTML文字列を作り、ブラウザが**静的に**描画する。この時点ではまだ**未ハイドレート**——「いいね」ボタンを押しても動かない(イベント未接続)。「ハイドレート」を押すと、既存DOMはそのままにイベントだけが付き、ボタンが反応するようになる。SSR とハイドレーションの間にある「見えるが動かない」瞬間を体感してほしい。

<MiniNextDemo />

## 繋ぐ: renderRoute と hydrateRoute

サーバ側は「URL → HTML」、クライアント側は「URL → 既存DOMにイベント付与」。同じルータと同じページ関数を両側で使う:

<<< ../../frontend/mini-next/app.ts#app{ts}

サーバとクライアントで**同じ木**を作るのが前提。ここがズレると、サーバの `<h1>A</h1>` にクライアントが `<h1>B</h1>` のつもりでイベントを付ける、といった食い違い(hydration mismatch)が起きる。

## アーキテクチャ面接の観点: なぜ SSR とハイドレーションなのか

「SPA でいいのに、なぜサーバでも描くのか」に答えられるか:

- **初回表示(FCP)と SEO**: SPA は「空の HTML + 大きな JS」から始まり、JS を落として実行するまで何も見えない。SSR は最初から中身のある HTML を返すので、速く見え、クローラも読める
- **ハイドレーションのコスト**: だが SSR HTML を送っても、対話可能にするには結局クライアントで同じ木を作ってイベントを付ける。この**二度手間**が TTI(操作可能までの時間)を押し上げる。「見えるのに押せない」時間が生まれる
- **部分/選択的ハイドレーション**: 全部を一度に hydrate せず、必要な島だけを hydrate する(Islands / React Server Components)。この章は一括 hydrate なので、そこは踏み込まない
- **サーバ/クライアントの同型性**: 同じコンポーネントを両側で実行するので、`window` 依存のコードがサーバで壊れる、といった落とし穴がある

面接では「SSR は初回表示と SEO のため。だがハイドレーションという二度手間が付きまとい、そこを削るのが最近の潮流(ストリーミング・部分ハイドレーション・サーバコンポーネント)」を押さえられるかが要。

## メリット・デメリットと実例

| 方式 | 初回HTML | 対話可能まで | 向く場面 | 実例 |
|---|---|---|---|---|
| CSR(SPA) | 空 + JS | JS実行後 | 管理画面など SEO 不要 | 素の React/Vue SPA |
| SSR + hydrate | 中身あり | hydrate後 | コンテンツ + 対話 | Next.js(Pages)、Nuxt |
| SSG | 事前生成の中身 | hydrate後 | 更新が稀なサイト | Next.js(SSG)、Astro、VitePress |
| Islands / RSC | 中身あり | 島だけ hydrate | 大半が静的なページ | Astro Islands、React Server Components |

裏どり:

- **Next.js**: この章の `renderToString` + ルーティング + ハイドレーションを、はるかに作り込んだもの。データ取得・コード分割・画像最適化などが乗る
- **VitePress(この教科書自身)**: SSG。ビルド時に各ページを HTML 化し、ブラウザで Vue が hydrate する。まさにこの章の仕組みの上で動いている
- **Astro**: 既定は JS ゼロの静的HTML。対話が要る部分だけを「島」として hydrate する(部分ハイドレーション)
- **React Server Components**: サーバでしか動かないコンポーネントを混ぜ、クライアントに送る JS を減らす新しい方向

## 簡略化したこと

- **状態管理・再描画なし**: hydrate 後の状態変化→再描画のループは無し(前章 vdom の `diff` を繋げば作れる)。ここは「静的HTML→対話可能」への接続まで
- **hydration mismatch の検出なし**: サーバとクライアントで木がズレたときの警告・修復は無し
- **ネストルート・レイアウト・データ取得なし**: `getServerSideProps` 的なデータ取得、ネストレイアウト、コード分割は無し
- **単純な `:id` 捕獲のみ**: `[...slug]` のワイルドカードやオプショナルセグメントは無し
- **ストリーミング・部分ハイドレーションなし**: 一括で文字列化・一括で hydrate

## 参考資料

- Next.js: [Rendering](https://nextjs.org/docs/app/building-your-application/rendering) — SSR/SSG/RSC の整理
- React: [`hydrateRoot`](https://react.dev/reference/react-dom/client/hydrateRoot) と hydration mismatch
- Astro: [Islands Architecture](https://docs.astro.build/en/concepts/islands/)
- 実装: [frontend/mini-next](https://github.com/esh2n/sharin/tree/main/frontend/mini-next)
