<script setup>
import ReactivityDemo from '../components/ReactivityDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# リアクティビティ(signal)

> 実装: [`frontend/reactivity/`](https://github.com/esh2n/sharin/tree/main/frontend/reactivity) / 実行: `pnpm test`(frontend/)

<Summary>
仮想DOMは状態が変わるたびに木を丸ごと作り直し、前回との差分を当てる。だが変わらぬ部分まで作る無駄がある。signalベースのリアクティビティは逆の発想だ。状態そのものが「誰が自分を読んだか」を覚え、変わったときにその読み手だけを再実行する。木を比べる必要がない。signal・effect・computedを実装し、読んだ人だけ反応するきめ細かさ、依存が変わるまで再計算しないキャッシュを見る。
</Summary>

## この章で作るもの

[仮想DOM](/parts/vdom)は、状態が変わるたびに「あるべき木」を丸ごと作り直し、前回の木と比べて(diff)、変わった箇所だけを実DOMに当てた。宣言的で分かりやすく、十分速い。だが無駄もある。ボタンのカウントが 1 増えただけでも、コンポーネントの木を丸ごと一度作り、全体を比べる。変わっていない大部分も、作っては捨てている。

signal ベースのリアクティビティは、発想を逆にする。木を作り直して比べるのでなく、状態そのものに「自分が変わったら誰を更新すべきか」を覚えさせる。カウントの signal は、自分を読んだ関数(エフェクト)を購読者として記録しておく。カウントが変わったら、その購読者だけを再実行する。木の比較はいらない。変わった値に繋がっている箇所だけが、ピンポイントで動く。これがきめ細かな(fine-grained)リアクティビティで、Solid・Vue・Preact Signals の心臓部だ。この章では、その中核である signal・effect・computed を実装する。

<FigureBox caption="signal は自分を読んだエフェクトを覚える。値が変わると、その購読者だけを再実行する。木全体を比べる仮想DOMと対照的">

```
signal(count) ──読まれる──▶ effect A(count を表示)
      │        ──読まれる──▶ computed(count×2) ──▶ effect B
      │
   count.set(5) → 購読者 A と computed だけ再実行(B も computed 経由で)
                  count を読んでいない他のエフェクトは動かない
```

</FigureBox>

順に見ていく。

1. **signal は購読者を覚える**: 読まれたとき、実行中のエフェクトを購読者に登録する(依存追跡)
2. **effect は依存が変わると再実行**: 読んだ signal だけを購読し、それが変わったときだけ動く(きめ細かさ)
3. **computed はキャッシュする**: 依存が変わるまで再計算しない。読まれなければ計算もしない(遅延評価)

## ① signal: 値と購読者

まず signal を作る。値を 1 つ持ち、それに加えて「自分を読んだエフェクトの集合」を持つ。読まれたときに、今実行中のエフェクトを購読者に登録するのが要だ:

<<< ../../frontend/reactivity/reactivity.ts#signal{ts}

`get()` の中の `track` が依存追跡の心臓だ。グローバルな `activeEffect` は「今まさに実行中のエフェクト」を指す。エフェクトが実行中に signal を読むと、その signal は `activeEffect` を自分の購読者に加える。誰も実行中でなければ(`activeEffect` が null)、ただ値を返すだけで購読は張られない。`set()` は、値が実際に変わったときだけ、購読者を全部再実行する。同じ値をセットしたときは何もしない。テストで、signal を変えると読んだエフェクトが再実行され、同じ値なら再実行しないことを固定した。

## ② effect: 依存を張り直しながら再実行する

次にエフェクト。signal を読む関数を受け取り、即座に実行し、読んだ signal が変わるたびに再実行する。肝は、再実行のたびに前回の購読をすべて解除してから走らせることだ:

<<< ../../frontend/reactivity/reactivity.ts#effect{ts}

なぜ毎回購読を張り直すのか。動的な依存に対応するためだ。エフェクトが `flag ? a.get() : b.get()` のように、条件で読む signal を変えるとする。flag が true のときは a を購読し、false になったら b を購読すべきで、もう読まない a からは購読が外れてほしい。毎回 `cleanup` で全購読を解除し、実行のたびに実際に読んだ signal だけを購読し直せば、これが自然に実現する。テストで、読んでいない signal の変更では再実行されないこと(きめ細かさ)、条件で読む signal が変わると購読先が切り替わること(動的依存)を固定した。戻り値の関数を呼べば購読を解除して停止できる(dispose)。

## ③ computed: 遅延とキャッシュ

最後に computed。signal から派生した値(幅×高さ=面積、など)を表す。素朴には毎回計算すればいいが、それでは同じ値を何度読んでも計算し直してしまう。computed は、依存が変わったときだけ「汚れた」印を付け、次に読まれたときに初めて再計算してキャッシュする:

<<< ../../frontend/reactivity/reactivity.ts#computed{ts}

computed は signal とエフェクトの両方の顔を持つ。内部に `runner` というエフェクトを持ち、依存の signal が変わると `runner` が呼ばれるが、そこでは再計算せず `stale = true` の印を付けて自分の購読者に伝播するだけだ。実際の再計算は、次に `get()` されたときまで遅延する。読まれなければ計算もしない。一度計算したら `stale` が false になり、依存が変わるまでキャッシュを返す。テストで、依存が変わるまで再計算しないこと(2 回読んでも計算は 1 回)、作っただけでは計算しないこと(遅延評価)、computed を連鎖できることを固定した。この遅延とキャッシュが、派生値の多いUIで無駄な再計算を防ぐ。

### 動かす

下のデモは、signal・computed・effect の依存グラフを可視化する。ある signal を変えたとき、どのエフェクトが再実行され、どれが動かないか(きめ細かさ)、computed がいつ再計算されるか(キャッシュ)を見てほしい。

<ReactivityDemo />

## 設計の観点

- **きめ細かさ vs 仮想DOM**: signal は変わった箇所だけ更新するので、大きなUIで無駄が少ない。仮想DOMは木の比較にコンポーネント単位のコストがかかる。どちらも実用で、Solid/Svelte は signal 寄り、React は仮想DOM 寄り
- **依存追跡は実行時**: どの signal を読むかを、コードを解析せず実行して観測する。だから条件分岐で依存が変わっても自然に追随する。コンパイル時解析(Svelte)と実行時追跡(Solid/Vue)の二系統がある
- **computed は純粋であるべき**: 依存の signal から値を計算するだけにする。副作用を混ぜると、遅延評価やキャッシュで実行回数が読めなくなる
- **バッチ処理**: 複数の signal を続けて変えると、素朴にはエフェクトが複数回走る。実物は変更をまとめて 1 回にする batch を持つ
- **メモリとクリーンアップ**: 購読は signal とエフェクトの双方向参照。エフェクトを dispose しないと購読が残り、リークする。フレームワークはコンポーネント破棄時に自動 dispose する

## 対照と実例

| | 仮想DOM(React) | signal(Solid/Vue) |
|---|---|---|
| 更新の起点 | 状態変更 → 再レンダリング | signal 変更 → 購読者 |
| 変更検出 | 木を diff | 依存グラフをたどる |
| 更新の粒度 | コンポーネント単位 | signal 単位(細かい) |
| 派生値 | useMemo(手動) | computed(自動追跡) |
| 無駄 | 変わらぬ部分も再構築 | 変わった箇所だけ |

裏どり:

- **SolidJS**: signal・effect・memo(computed)を前面に出したフレームワーク。この章の API はこれに近い
- **Vue 3 Reactivity**: `ref` / `reactive` / `computed` / `watchEffect`。Proxy で読み取りを追跡する実行時方式
- **Preact Signals**: React にも signal を持ち込んだ実装。仮想DOMと signal の共存例
- **Svelte 5 Runes**: コンパイル時に依存を解析する方式。実行時追跡とは別のアプローチ

## 簡略化したこと

- **バッチ処理なし**: 複数の set をまとめる batch は省略。set ごとに即座にエフェクトが走る
- **同期実行**: 実物はマイクロタスクでまとめて次のフレームで反映することが多い
- **DOM 連携なし**: signal を DOM 更新に繋ぐ部分は扱わない(テンプレートや vdom の役目)
- **循環依存の検出なし**: 依存が輪を作ると無限ループしうる

## 参考資料

- [SolidJS: Reactivity](https://www.solidjs.com/tutorial/introduction_signals) — signal ベースのリアクティビティの入門
- [Vue: Reactivity in Depth](https://vuejs.org/guide/extras/reactivity-in-depth.html) — 依存追跡の仕組みの解説
- [Preact Signals](https://preactjs.com/guide/v10/signals/) — signal の設計と仮想DOMとの共存
- 実装: [frontend/reactivity](https://github.com/esh2n/sharin/tree/main/frontend/reactivity)
