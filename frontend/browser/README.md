# browser — レンダリングパイプライン

HTML文字列と CSS文字列から、画面に何をどこに描くか(描画コマンド)を計算する——
ブラウザのレンダリングエンジンの背骨を最小構成でフルスクラッチする。フロント編の締めくくり。

教科書の章: [browser](https://sharin-2a1.pages.dev/parts/browser)

## これは何か

[mini-next](../mini-next) の SSR は「木 → HTML文字列」だった。browser はその**逆から始まる**:
HTML文字列を DOM 木に戻し、CSS を当て、位置を計算し、描画コマンドにする。ブラウザが
アドレスバーに URL を入れてから画面が出るまでにやっていることの、核だけを取り出す。

パイプラインは5段:

```
HTML文字列 ──parse──▶ DOM木
CSS文字列  ──parse──▶ 規則         ┐
                                   ├─style─▶ スタイル木 ─layout─▶ レイアウト木 ─paint─▶ 描画コマンド
DOM木 ─────────────────────────────┘
```

- `parseHTML(html)` — 再帰下降パーサで HTML文字列 → DOM木(SSR の逆)
- `parseCSS(css)` — セレクタ + 宣言の規則配列に
- `styleTree(dom, sheet)` — DOM × CSS。詳細度でカスケード、color は継承
- `layout(styled, width)` — ブロックレイアウト。各ボックスの x/y/幅/高さ
- `paint(box)` — 背景矩形とテキストの描画コマンド一覧(ディスプレイリスト)
- `render(html, css, width)` — 上記を繋ぐパイプライン全体

## 肝は3つ

1. **段階に分ける**: parse → style → layout → paint。各段は前段の出力だけを入力に取る一方通行。だからそれぞれを単体で理解・テストできる
2. **スタイルはカスケード + 継承**: 同じプロパティが複数当たれば詳細度(id>class>tag)の高い規則が勝つ。color のような一部プロパティは子(テキスト含む)へ受け継ぐ
3. **レイアウトは箱の入れ子**: ブロックは親の幅を満たし、子は上から下へ積む。padding は内側へ、margin は外側へ。位置は「親のどこに、前の兄弟の下に」で決まる

## 設計メモ

- **HTMLパーサ**: 「現在位置」を持つ再帰下降。`<` を覗いて要素かテキストかを決めて食べ進む。閉じタグの対応が取れないと例外
- **詳細度**: `[id数, class数, tag数]` のタプルで比較。CSS の specificity の縮小版
- **継承**: color / font 系だけを子に渡す。background/margin など箱固有のものは渡さない
- **ブロックレイアウトのみ**: 縦積み。幅は明示 width か「親幅 − 左右margin」。高さは明示 height か子の合計、葉テキストは1行分
- **ペイント順**: 親を先に、子を後に積む(後のコマンドが手前に描かれる=正しい重なり)

## 簡略化したこと

- **インラインレイアウトなし**: テキストの行分割・折り返し・inline要素の横並びは無し。テキストは1行の箱として扱う
- **ボックスモデルは単一px**: `margin: 10px` のような四辺一律のみ。`10px 20px` や auto、border 幅は無し
- **セレクタは単純セレクタのみ**: 子孫結合子 `a b`、擬似クラス `:hover`、属性セレクタは無し
- **色・単位は文字列のまま**: `#f00` や `red` を解釈せずそのまま描画コマンドに載せる。`em`/`%` 等の相対単位は無し
- **実描画はしない**: paint はコマンド一覧を返すところまで。実際のラスタライズ(GPU/canvas)は範囲外
- **HTMLパーサは最小**: コメント・DOCTYPE・void要素・エンティティ・不正入力の寛容な回復は無し

## 動かす

```bash
cd frontend
pnpm test        # vitest
pnpm cov         # カバレッジ
pnpm typecheck   # tsc --noEmit（strict）
```

このMacでは `mise exec node@22.20.0 -- npx pnpm@9.15.0 <cmd>`。

## 参考

- Matt Brubeck, [Let's build a browser engine!](https://limpet.net/mbrubeck/2014/08/08/toy-layout-engine-1.html)(robinson) — この実装の下敷き
- Tali Garsiel, [How Browsers Work](https://web.dev/articles/howbrowserswork)
