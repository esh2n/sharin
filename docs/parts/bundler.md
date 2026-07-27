<script setup>
import BundlerDemo from '../components/BundlerDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# バンドラ(依存解決)

> 実装: [`frontend/bundler/`](https://github.com/esh2n/sharin/tree/main/frontend/bundler) / 実行: `pnpm test`(frontend/)

<Summary>
アプリは何百ものモジュールに分かれ、importで依存し合う。ブラウザにそれを個別に読ませるのは非効率だ。バンドラは入口から importを辿って依存グラフを作り、依存が先に来るよう並べ替えて1つにまとめる。import が輪を作っていれば循環依存として検出する。依存グラフの探索・トポロジカル順序・循環検出・使われないexportを落とすtree-shakingを実装する。
</Summary>

## この章で作るもの

コードをモジュールに分けるのは良い設計だ。1 つの関心事を 1 つのファイルに閉じ込め、必要なものだけを import する。だがこの「良い設計」は、そのままではブラウザに優しくない。何百ものファイルを個別に取りに行くのは遅く、依存の順序(どのファイルを先に読むべきか)も自明ではない。バンドラは、この開発時の理想と実行時の効率の間を埋める。

やることは 3 段階だ。まず、入口(entry)のモジュールから import を辿り、到達できるモジュールをすべて集めて依存グラフを作る。次に、そのグラフを並べ替える。あるモジュールが別のモジュールを import しているなら、import される側が先に定義されていなければならない。これはグラフのトポロジカル順序だ。並べる過程で、import が輪を作っている(A が B を、B が A を import する)循環依存も検出できる。最後に、依存グラフを見れば「どの export が実際に使われているか」が分かるので、どこからも import されない export を削る。これが tree-shaking だ。この章では、この依存解決の中核を実装する。

<FigureBox caption="バンドラの依存解決。entry から import を辿ってグラフを作り、依存が先に来るよう並べ替え、使われない export を落とす">

```
entry ─import{add}─▶ math (add, sub, mul)
  │                        ▲
  └─import{log}─▶ util ─import{sub}┘

到達: entry, math, util (orphan は含まれない)
順序: math → util → entry (依存が先)
tree-shake: math の mul は誰も使わない → 落とす
```

</FigureBox>

肝は3つ:

1. **依存グラフを辿る**: entry から import を辿り、到達できるモジュールだけを集める。使われないモジュールは含めない
2. **トポロジカル順序**: 依存を先に、使う側を後に並べる。DFS の帰りがけ順がそれになる。循環なら検出
3. **tree-shaking**: 実際に import される export だけ残し、使われない export を落とす

## ① 依存グラフ: 到達できるものだけ集める

まず entry から import を辿って、バンドルに含めるべきモジュールを集める。グラフの到達可能性の探索だ:

<<< ../../frontend/bundler/bundler.ts#graph{ts}

`collectReachable` は entry から出発し、各モジュールの import 先を辿って、到達できるモジュール ID をすべて集める。ここで大事なのは、どこからも import されないモジュールは集合に入らないことだ。プロジェクトに 500 ファイルあっても、entry から辿れるのが 200 なら、バンドルに含まれるのはその 200 だけ。使われないコードは最初から運ばない。テストで、entry から辿れる math・util は集まり、どこからも import されない orphan は含まれないことを固定した。これがバンドルの土台になるグラフだ。

## ② トポロジカル順序: 依存を先に並べる

集めたモジュールを、1 つのファイルに連結する順序を決める。ルールは単純で、あるモジュールが import するモジュールは、そのモジュールより先に来なければならない。使う前に定義されている必要があるからだ。これはグラフのトポロジカル順序で、DFS の帰りがけ順で得られる:

<<< ../../frontend/bundler/bundler.ts#topo{ts}

`visit` は、あるモジュールの import 先を全部訪ね終えてから、自分を `order` に積む。だから依存が先に、それを使う側が後に並ぶ。同時に循環も検出する。ノードに 3 色(未訪問・訪問中 gray・完了 black)を付け、訪問中の gray なノードに戻ってきたら、それは import が輪を作っている証拠だ。今辿ってきた経路から循環部分を切り出して `CycleError` で返す。テストで、依存の math が使う側の util・entry より前に並ぶこと、a→b→c→a の循環が経路つきで検出されることを固定した。循環依存は、初期化順序が定まらず実行時エラーの温床になるので、バンドル時に見つけて警告できるのは大きい。

## ③ tree-shaking: 使われない export を落とす

最後に無駄を削る。依存グラフには「どのモジュールが、どのモジュールから、どの名前を import しているか」の情報がある。これを逆に見れば、各モジュールの export のうち、実際に誰かに import されているものが分かる。どこからも import されない export は、あっても使われない死んだコードだ。落とせる:

<<< ../../frontend/bundler/bundler.ts#treeshake{ts}

`treeShake` は、到達可能な全モジュールの import を走査して「使われる名前」を集め、各モジュールの export をそれだけに絞る。math が `add`(entry から)と `sub`(util から)を使われ、`mul` は誰も使わないなら、`mul` は落ちる。entry 自身はアプリの入口で、その export はアプリのコードそのものなので全部残す。テストで、誰も使わない `mul` が落ち、使われる `add`・`sub` が残ること、どの名前も import されないモジュールは export が空になることを固定した。tree-shaking が効くには、モジュールが静的に解析できる(import/export が実行時に変わらない)ことと、副作用がないことが前提になる。だから ES Modules の静的な import 構文が重要で、これが動的だと何が使われるか静的には分からず、削れない。

### 動かす

下のデモは、モジュールの依存グラフを与えて、到達できるモジュール・連結の順序・使われない export の削除を段階的に見る。孤立モジュールが除かれ、依存が先に並び、死んだ export が落ちる様子を確かめてほしい。

<BundlerDemo />

## 設計の観点

- **静的解析が前提**: tree-shaking も依存グラフも、import/export が静的に読めることに依存する。動的 import や実行時に決まる依存は解析できず、削れない。ES Modules の静的構文がこれを可能にした
- **副作用の扱い**: import しただけで副作用(グローバル登録など)があるモジュールは、export が未使用でも消せない。バンドラは `sideEffects` の指定で副作用の有無を知る
- **循環依存は警告する**: 循環は初期化順序を不定にする。バンドルは通せても実行時にundefinedを掴む危険があるので、検出して警告するのが親切
- **コード分割**: すべてを 1 つに固めず、動的 import の境界でチャンクに分ける。初期ロードを軽くする。この章の静的グラフの先にある発展
- **開発と本番の違い**: 開発時は速い再ビルド(esbuild/Vite の no-bundle)、本番は最適化重視(tree-shaking, minify)。同じ依存解決でも目的で手法が変わる

## 対照と実例

| 段階 | やること | 効果 |
|---|---|---|
| 依存解決 | import を辿ってグラフ化 | 使うモジュールだけ集める |
| 順序付け | トポロジカルソート | 依存を先に連結 |
| 循環検出 | DFS の gray 検出 | 初期化順序の罠を警告 |
| tree-shaking | 未使用 export の除去 | バンドルを小さく |

裏どり:

- **webpack**: 依存グラフからバンドルを組む定番。ローダーとプラグインで変換を挟む
- **Rollup**: tree-shaking を前面に出した先駆。ライブラリ配布向けの小さなバンドル
- **esbuild / Vite**: 高速な依存解決とバンドル。Vite は開発時に no-bundle(ブラウザの ES Modules を活用)
- **ES Modules (RFC/仕様)**: 静的な import/export。tree-shaking が可能になった土台

## 簡略化したこと

- **パースなし**: 実物はソースを AST に解析して import/export を抽出する。ここは宣言済みの依存グラフを受け取る
- **副作用の考慮なし**: 副作用のある import は本来 tree-shake できない。ここは純粋前提
- **コード生成なし**: 順序と生存 export を出すまで。実際の連結・変換・minify・ソースマップは扱わない
- **動的 import なし**: `import()` によるコード分割やチャンク生成は扱わない

## 参考資料

- [webpack: Dependency Graph](https://webpack.js.org/concepts/dependency-graph/) — 依存グラフからのバンドル
- [Rollup: Tree-shaking](https://rollupjs.org/introduction/#tree-shaking) — 未使用コード除去の仕組み
- [MDN: JavaScript modules](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Guide/Modules) — 静的 import/export の基礎
- 実装: [frontend/bundler](https://github.com/esh2n/sharin/tree/main/frontend/bundler)
