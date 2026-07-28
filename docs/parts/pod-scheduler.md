<script setup>
import PodSchedulerDemo from '../components/PodSchedulerDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# スケジューラ(Podの配置)

> 実装: [`orchestration/scheduler/`](https://github.com/esh2n/sharin/tree/main/orchestration/scheduler) / 実行: `go test ./orchestration/scheduler/`

<Summary>
調整ループが作った Pod は、まだどこにも置かれていない。それをどのマシンに載せるか決めるのがスケジューラだ。決め方は2段階で、まず filter が置けないノードを落とし、次に score が残った候補に点をつける。可否と優劣を分けるのが肝になる。点の付け方を変えるだけで、負荷を散らす配置にも詰め込む配置にもなる。スケジューラを実装し、同じ Pod が戦略しだいで別のノードに落ちる様子を見る。
</Summary>

## この章で作るもの

[調整ループ](/parts/reconcile)は「Pod を 3 個にせよ」までを決めた。だが作られた Pod はまだどのマシンにも載っていない。Pending のまま宙に浮いている。クラスタには容量の違うノードが何台もあり、GPU が刺さった特殊なノードもある。どの Pod をどのノードに置くか。これを決めるのがスケジューラで、Kubernetes では kube-scheduler がやる。この章ではそれを作る。

置き方の良し悪しは、ここでほぼ決まってしまう。全部を 1 台に詰め込めば、そのノードが落ちたとき全滅する。逆に均等に散らせば、どのノードも半端に埋まって、大きな Pod が 1 つも入らなくなる。GPU ノードに普通の Web サーバを置けば、高価な資源が遊ぶ。つまりスケジューラは、置ける場所を絞る仕事と、置ける中から選ぶ仕事の 2 つを兼ねている。Kubernetes はこの 2 つを、filter と score という別々の段に分けた。

<FigureBox caption="スケジューリングの2段階。filter で置けないノードを落とし、残った候補に score で点をつける。点の付け方を変えても、置ける場所そのものは変わらない">

```
  Pod(要求 500m/512Mi)
        │
        ▼
  ┌── ① filter ──────────────┐   置けるか(可否)
  │ 空きが足りるか            │   node-a  空き 2000m  → 残る
  │ 汚れを許容しているか      │   node-b  空き  200m  → 落ちる
  └────────┬─────────────────┘   gpu-1   汚れ有り    → 落ちる
           ▼
  ┌── ② score ───────────────┐   どこが良いか(優劣)
  │ 置いた後の使用率で採点    │   Spread  空きが多いほど高得点
  └────────┬─────────────────┘   BinPack 埋まるほど高得点
           ▼
  最高点のノードに bind(要求のぶんを予約)
```

</FigureBox>

肝は3つ:

1. **可否と優劣を分ける**: filter は「置けるか」だけ、score は「どこが良いか」だけを見る
2. **要求(request)で予約する**: 実際の使用量でなく、Pod が申告した要求のぶんを確保する
3. **戦略は score だけを変える**: 散らすか詰めるかを変えても、置ける場所は変わらない

## ① filter: 置けるノードを絞る

まず、Pod を置けないノードを落とす。判定するのは 2 つ。要求した CPU とメモリが空きに収まるか、そしてノードに付いた汚れ(taint)を許容しているか:

<<< ../../orchestration/scheduler/scheduler.go#filter{go}

`Filter` は候補のリストと、各ノードの判定を並べて返す。ここで返しているのが可否だけである点が大事だ。落ちたノードは候補から消え、通ったノードは全部が対等に残る。優劣は一切つけない。

汚れは、ノード側から「特別な事情がある」と表明する仕組みだ。GPU ノードに `hardware=gpu` という汚れを付けると、それを許容する(tolerate)と宣言した Pod しか置けなくなる。普通の Web サーバは何も許容していないので、GPU ノードには載らない。高価なノードを、それを必要とする Pod のために空けておける。テストで、同じ Pod でも許容を足すだけで置けるようになることを固定した。

落ちた理由を残しているのも意図がある。Pod がいつまでも Pending のとき、知りたいのは「なぜ置けないのか」だ。空きが足りないのか、汚れなのか。実物の Kubernetes も `kubectl describe pod` でこの理由を並べて見せる。理由を捨てると、Pending の原因が追えなくなる。

## ② score: 候補に順位をつける

filter を通ったノードは、どれも置ける。その中からどれを選ぶか。ここで点をつける:

<<< ../../orchestration/scheduler/scheduler.go#score{go}

計算しているのは「その Pod を置いた後の使用率」1 つだけで、それをどう読むかが戦略の違いになる。`BinPack` は使用率をそのまま点にする。埋まるノードほど高得点なので、1 台が満杯になるまで詰め、他は空のまま残る。`Spread` は 100 から引く。空いているノードほど高得点なので、Pod は均等に散る。

この違いは効き方が正反対だ。詰めれば、空のノードをそのまま落とせるのでコストが下がる。クラウドで台数課金されるならこれが効く。だが 1 台に集中しているので、そのノードが死ぬと被害が大きい。散らせば障害の影響は小さくなるが、どのノードも半端に埋まり、大きな Pod が入る隙間がなくなる。どちらが正しいということはなく、何を守りたいかで決まる。Kubernetes の既定は散らす側で、詰める側は明示的に選ぶ。テストで、同じ 3 つの Pod と同じ 3 台のノードに対し、戦略だけを変えると均等配置と 1 台集中に分かれることを固定した。

そして、この切り替えが score だけで済むことに意味がある。filter は共通なので、戦略を変えても「置けない場所に置く」ことは絶対に起こらない。守るべき制約と、好みの方針が、コードの上でも分かれている。

## ③ bind: 要求のぶんを予約する

最後に、最高点のノードに Pod を束縛(bind)する:

<<< ../../orchestration/scheduler/scheduler.go#schedule{go}

`Schedule` は filter・score・bind の 3 段をそのまま並べただけだ。候補が 1 つも残らなければ何もしない。Pod は Pending のまま残る。Kubernetes も同じで、置けない Pod を無理にどこかへ押し込んだりはしない。ノードが増えるか、他の Pod が消えて空きが出るまで、待ち続ける。

bind するとき、ノードの使用量に足すのは Pod が申告した要求(request)であって、実測値ではない。ここが実運用で効いてくる。要求は予約であり、Pod が実際にどれだけ使うかとは関係しない。500m と申告して 50m しか使わない Pod でも、ノードの空きは 500m 減る。だから申告が実態より大きいと、ノードは空いているのに新しい Pod が入らない。逆に小さすぎると、詰め込みすぎて全員が遅くなる。要求をどう決めるかは、スケジューラの外側にある運用の問題だが、スケジューラの挙動を決めているのは要求の値だ。テストで、配置すると要求のぶんだけ空きが減ることと、容量ぴったりまで詰めると次が入らなくなることを固定した。

同点の扱いも決めておく。空のノードが 3 台あって全部同点のとき、どれを選んでも構わないが、選び方が毎回変わるとテストが書けない。ここはノード名の辞書順で決めた。実物のスケジューラは同点をランダムに散らして偏りを避けるが、本章は決定性を優先している。

### 動かす

下のデモは、3 台のノードに Pod を置いていく。戦略を切り替えると score の欄だけが変わり、filter の欄は変わらないことを確かめてほしい。GPU ノードは汚れが付いているので、普通の Pod は落ちる。小さい Pod をいくつか置いてから大きい Pod を置くと、空き不足で filter に落ちて Pending になる。

<PodSchedulerDemo />

## 設計の観点

- **filter と score の分離**: 制約(置けない)と方針(置きたくない)を混ぜない。混ぜると、方針を変えたときに制約が壊れる。プラグインとして両方を差し替えられる構造が、実物の scheduler framework の骨格になっている
- **要求は予約、実測ではない**: スケジューラは申告値だけを見る。実測を見て詰めるのは別の仕組み(VPA や descheduler)の仕事で、スケジューラ自身は単純さを保つ
- **1 つずつ順に処理する**: まとめて最適配置を解くのでなく、Pod を 1 つずつ処理する。全体最適は諦めているが、その代わり速く、Pod が増えても破綻しない。先に置いた Pod が後の判断を変えるので、順序が結果を変える
- **置けなければ待つ**: 無理に置かず Pending のまま残す。ノードが増えるか空きが出れば置ける。[調整ループ](/parts/reconcile)が Pod を作り続けるのと同じで、いつか置ければよいという設計
- **preemption は別の層**: 実物は、優先度の高い Pod のために低い Pod を追い出せる。これは filter でも score でもなく、両方が失敗した後の最後の手段として置かれている

## 対照と実例

| | filter(predicates) | score(priorities) |
|---|---|---|
| 決めること | 置けるか | どこが良いか |
| 結果 | 可否(残るか落ちるか) | 点数(順位) |
| 変えると | 置ける場所が変わる | 選ぶ場所が変わる |
| 例 | 空き容量、汚れ、affinity | 使用率、Pod の分散 |
| 失敗すると | Pod が Pending のまま | 偏った配置になる |

裏どり:

- **kube-scheduler**: filter と score の 2 段構成、およびプラグインとして拡張する scheduler framework の設計
- **Taints and Tolerations**: ノード側から Pod を弾く仕組み。ノード affinity(Pod 側からノードを選ぶ)との役割の違い
- **NodeResourcesFit プラグイン**: LeastAllocated(散らす)と MostAllocated(詰める)の切り替え。本章の Spread / BinPack にあたる
- **Resource requests and limits**: request がスケジューリングに使われ、limit が実行時の上限になるという役割の分離

## 簡略化したこと

- **predicates は 2 つだけ**: 実物は node affinity、ポートの衝突、ボリュームの制約、topology spread など多数を順に当てる
- **preemption なし**: 優先度の低い Pod を追い出して場所を空ける機能は扱わない
- **キューと再試行なし**: 実物は置けなかった Pod をキューに戻し、状況が変わったら再試行する
- **taint の effect なし**: 実物の汚れは NoSchedule / PreferNoSchedule / NoExecute を持つ。ここは NoSchedule 相当のみ
- **オートスケールなし**: 置けない Pod が続いたらノードを増やすのが本来の答え。何個であるべきかを決める側は次章([水平オートスケール](/parts/autoscaler))で扱う

## 参考資料

- [Kubernetes: Scheduler](https://kubernetes.io/docs/concepts/scheduling-eviction/kube-scheduler/) — filter と score の 2 段構成
- [Scheduling Framework](https://kubernetes.io/docs/concepts/scheduling-eviction/scheduling-framework/) — 拡張点の設計
- [Taints and Tolerations](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) — ノード側から弾く仕組み
- 実装: [orchestration/scheduler](https://github.com/esh2n/sharin/tree/main/orchestration/scheduler)
