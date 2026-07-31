<script setup>
import BrowserDemo from '../components/BrowserDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# ブラウザ(レンダリングパイプライン)

<Summary>
HTML 文字列と CSS 文字列から「画面のどこに何を描くか」を計算する、ブラウザのレンダリングエンジンの背骨を最小構成で作る。処理は parse → style → layout → paint の一方通行で、各段は前段の出力だけを入力に取るので単体でテストできる。スタイルは詳細度の高い規則が勝つカスケードと、color などを子へ渡す継承で決まる。レイアウトは箱の入れ子で、ブロックは親幅を満たし子を上から下へ積む。
</Summary>

## この章で作るもの

[mini-next](./mini-next) の SSR は「仮想DOM木 → HTML文字列」だった。ブラウザはその逆から始める。受け取った HTML文字列を DOM 木に戻し、CSS を当て、各要素の位置と大きさを計算し、最後に「ここに この色の矩形」「ここに この文字」という描画コマンドにする。URL を入れてから画面が出るまでの、レンダリング部分の核を取り出す。

パイプラインは5段の一方通行:

<FigureBox caption="レンダリングパイプライン。HTML と CSS は別々にパースされ、style で合流する。以降 layout → paint と一方向に流れ、各段は前段の出力だけを入力に取る。だから段ごとに切り離して理解・テストできる。SSR(mini-next)が『木→文字列』だったのに対し、ここは『文字列→木→…→画面』">

```
  HTML文字列 ──parse──▶ DOM木 ─────────┐
                                        ├─ style ─▶ スタイル木
  CSS文字列  ──parse──▶ 規則(セレクタ+宣言)┘   (カスケード+継承)
                                                    │
                                                  layout  (位置と大きさ)
                                                    │
                                                    ▼
                                              レイアウト木
                                                    │
                                                  paint   (描画コマンド化)
                                                    │
                                                    ▼
                                          描画コマンド一覧 ─▶ 画面
```

</FigureBox>

先に押さえることが3つある。

1. **段階に分ける**: parse → style → layout → paint。各段は前段の出力だけを入力に取る。だから単体で理解・テストできる
2. **スタイルはカスケード + 継承**: 詳細度の高い規則が勝ち、color などは子(テキスト含む)へ受け継ぐ
3. **レイアウトは箱の入れ子**: ブロックは親幅を満たし、子は縦に積む。位置は「親のどこに、前の兄弟の下に」で決まる

## ① HTMLパース: 文字列 → DOM木

SSR の逆。文字列を先頭から食べ進む再帰下降パーサで木を組む。`<` を覗いて要素かテキストかを決める:

<<< ../../frontend/browser/html.ts#parser{ts}

`parseElement` が「開きタグ → 属性 → 子(閉じタグまで) → 閉じタグ」を順に食べ、子の `parseNodes` が中で再び `parseNode` を呼ぶ。この再帰が入れ子構造を作る。閉じタグが対応しなければ `expect` が例外を投げる(実ブラウザは寛容に回復するが、ここは投げる)。

## ② CSSパース: 文字列 → 規則

`セレクタ { 宣言 }` を規則の配列にする。セレクタは `tag` / `.class` / `#id` の単純セレクタのみ:

<<< ../../frontend/browser/css.ts#parse{ts}

決め手は**詳細度(specificity)**だ。`#id` は class より、class は tag より強い。これを `[id数, class数, tag数]` のタプルで表し、比較する。後で同じプロパティが競合したとき、この強さで勝敗を決める:

<<< ../../frontend/browser/css.ts#types{ts}

## ③ スタイル計算: DOM × CSS → スタイル木

各要素に、マッチした宣言を貼り付ける。同じプロパティが複数当たったら**詳細度の高い規則が後勝ち**(カスケード):

<<< ../../frontend/browser/style.ts#match{ts}

もう一つが**継承**。`color` を親に指定すると、子や中のテキストにも伝わる。一方 `background` や `margin` は箱固有なので伝わらない。継承対象だけを子へ渡す:

<<< ../../frontend/browser/style.ts#tree{ts}

## ④ レイアウト: スタイル木 → 位置と大きさ

ここで初めて数値(x, y, 幅, 高さ)が決まる。ブロックレイアウトは単純で強力な規則に従う。ブロックは親の幅を満たし、子は上から下へ積む:

<<< ../../frontend/browser/layout.ts#layout{ts}

- **幅**: 明示 `width` があればそれ、無ければ「親のコンテンツ幅 − 左右 margin」で満たす
- **位置**: `x` は親のコンテンツ左端 + margin、`y` は親のコンテンツ上端 + 前の兄弟までの高さ + margin
- **高さ**: 明示 `height`、無ければ子の合計。葉のテキストは1行分
- padding は内側(自分の高さを増やし子を内に寄せる)、margin は外側(隣との隙間)

「幅は親から下りてきて、高さは子から上がってくる」。この向きの違いがブロックレイアウトの勘所だ。

## ⑤ ペイント: レイアウト → 描画コマンド

最後に、位置の決まった箱を「描画コマンドの一覧(ディスプレイリスト)」にする。実ブラウザはこのリストを GPU に渡す。ここでは背景の矩形とテキストだけを出す:

<<< ../../frontend/browser/paint.ts#paint{ts}

親を先に、子を後に積むのが要だ。後のコマンドが手前に描かれるので、親の背景の上に子が正しく重なる。

### 動かす

下のデモは、`frontend/browser` の `render(html, css, width)` をそのまま呼んでいる。左に入力の HTML/CSS と、パイプラインが吐いた**描画コマンド一覧**。右は、そのコマンドを**そのまま矩形と文字として描いた結果**だ。座標も色もパイプラインの計算値をそのまま使っている。サンプルを切り替えて、CSS の padding / margin / height がコマンドの座標にどう効くかを見てほしい。

<BrowserDemo />

これで、HTML と CSS の文字列から実際に「箱の絵」が出るところまで繋がった。ここまでがブラウザのレンダリングの背骨。

## 設計の観点: なぜ段に分け、なぜ再レイアウトが重いのか

- **段に分ける理由**: parse/style/layout/paint を分けると、各段を並列化・キャッシュ・部分再実行できる。実ブラウザは「スタイルだけ変わった」「レイアウトも変わる」を区別して、必要な段だけやり直す
- **reflow(再レイアウト)が高い**: `width` や `height` を変えると layout からやり直しになり、子孫全体の位置を再計算する。対して `color` や `background` だけなら paint からで済む。だから「レイアウトを触る変更」は重い
- **レイアウトスラッシング**: JS で「スタイルを変える → 位置を読む → また変える」を繰り返すと、読むたびに強制同期レイアウトが走って大きく遅くなる。読みと書きをまとめろ、の理由がこれ
- **合成(compositing)**: `transform` や `opacity` のアニメは layout/paint を飛ばして合成だけで動かせる(GPU)。だから「位置を動かすなら `left` より `transform`」と言われる

## メリット・デメリットと実例

| 段 | 入力 → 出力 | やり直しの引き金 | 実ブラウザでの相当 |
|---|---|---|---|
| parse | 文字列 → 木 | 文書の変更 | HTMLパーサ、CSSパーサ |
| style | DOM × CSS → スタイル木 | class 変更、CSS 追加 | Style calculation |
| layout | スタイル木 → 位置 | width/height/位置系の変更 | Layout / reflow |
| paint | レイアウト → コマンド | 色・背景の変更 | Paint |
| (合成) | コマンド → 画面 | transform/opacity | Compositing(GPU) |

裏どり:

- **Blink(Chrome) / WebKit(Safari) / Gecko(Firefox)**: いずれもこの parse→style→layout→paint→composite の骨格を持つ。各段が桁違いに作り込まれている(インクリメンタル更新、複数スレッド、GPU 合成)
- **robinson**: Matt Brubeck の教育用エンジン。この章の下敷きで、Rust で同じ5段を最小実装している
- **React / Vue の「仮想DOM」との関係**: あれは[このパイプラインの手前](./vdom)にあたり、「DOM をどう変えるか」を決める層。DOM が変われば、その先でブラウザがこのパイプラインを回す

## 簡略化したこと

- **インラインレイアウトなし**: テキストの行分割・折り返し、inline 要素の横並びは無し。テキストは1行の箱として扱う
- **ボックスモデルは単一px**: `margin:10px` の四辺一律のみ。`10px 20px`・auto・border 幅は無し
- **セレクタは単純セレクタのみ**: 子孫結合子・擬似クラス・属性セレクタは無し
- **単位・色を解釈しない**: `#f00`/`red` をそのまま描画コマンドに載せる。`em`/`%` 等の相対単位は無し
- **実描画・合成なし**: paint はコマンド一覧まで。ラスタライズ(canvas/GPU)と合成は範囲外
- **インクリメンタル更新なし**: 毎回全段を通す。実ブラウザの「必要な段だけやり直す」最適化は無し

## 参考資料

- Matt Brubeck, [Let's build a browser engine!](https://limpet.net/mbrubeck/2014/08/08/toy-layout-engine-1.html)(robinson) — この章の下敷き
- Tali Garsiel, [How Browsers Work](https://web.dev/articles/howbrowserswork) — レンダリングパイプラインの定番解説
- [Render-tree Construction, Layout, and Paint](https://web.dev/articles/critical-rendering-path/render-tree-construction)(Critical Rendering Path)
- 実装: [frontend/browser](https://github.com/esh2n/sharin/tree/main/frontend/browser)
