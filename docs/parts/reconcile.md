<script setup>
import ReconcileDemo from '../components/ReconcileDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# 調整ループ(reconciliation)

> 実装: [`orchestration/reconcile/`](https://github.com/esh2n/sharin/tree/main/orchestration/reconcile) / 実行: `go test ./orchestration/reconcile/`

<Summary>
Kubernetesは宣言で動く。「Podは3個であれ」と宣言すれば、コントローラが宣言と現状を見比べて差を埋める。足りなければ作り、多ければ消す。これが調整ループだ。肝は宣言的であることと、level-triggeredであること。イベントに反応せず毎回現状を数え直すので、取りこぼしても必ず追いつく。調整ループを実装し、作成・スケール・障害回復を1つのループが等しく処理する様子を見る。
</Summary>

## この章で作るもの

[コンテナ](/parts/container)で namespace と cgroup による 1 プロセスの隔離を作った。だが本番では、コンテナを何十何百と、複数のマシンに散らして動かし、落ちたら立て直し、負荷に応じて増減させる必要がある。これを担うのがコンテナオーケストレータで、その代表が Kubernetes だ。Kubernetes には多くの部品があるが、その全体を貫く 1 つの発想がある。調整ループ(reconciliation loop)だ。この章は、まずそこから作る。

普通のシステム管理は手続き的だ。「Pod を 1 個起動する」「もう 1 個起動する」と、やることを順に命じる。Kubernetes は逆で、宣言的だ。人が書くのは「あるべき状態(desired state)は Pod 3 個」という状態だけ。どうやって 3 個にするかは書かない。その差を埋めるのがコントローラで、宣言された desired state と、今まさにある observed state を、ひたすら見比べる。3 個であるべきなのに 2 個しかなければ 1 個作る。4 個あれば 1 個消す。この「見比べて差を埋める」を繰り返すのが調整ループだ。作成も、スケールも、障害からの回復も、すべてこの 1 つのループが処理する。

<FigureBox caption="調整ループ。desired(あるべき数)と observed(今ある数)を毎回見比べ、差を埋める操作を出す。作成・スケール・自己修復がすべて同じ処理に集約される">

```
   desired = 3          observed を数える
       │                     │
       ▼                     ▼
   ┌── 見比べる ──┐   足りない → 作る
   │ desired vs   │   多い     → 消す
   │ observed     │   同じ     → 何もしない(冪等)
   └──────┬───────┘
          ▼
   observed が変わる → また見比べる(ループ)
```

</FigureBox>

順に見ていく。

1. **宣言的**: 「3 個であってほしい」という状態を宣言する。手順(どう 3 個にするか)は書かない
2. **level-triggered**: イベントに反応せず、毎回まるごと現状を数え直して差を埋める。取りこぼしに強い
3. **1 ループが全部やる**: 作成・スケール・障害回復が、同じ「差を数えて埋める」処理に集約される

## ① observed state: 今ある状態

まず、調整の対象になる世界を作る。クラスタは Pod の集まりだ。Pod は Kubernetes の実行単位で、1つ以上のコンテナをひとまとめにして起動・停止する箱になる。各 Pod は状態(Pending / Running / Failed)を持つ。これが observed state、つまり「今まさにある状態」だ:

<<< ../../orchestration/reconcile/reconcile.go#types{go}

Pod は作られるとまず Pending(起動待ち)になり、起動すると Running になる。ノードの障害やクラッシュで Failed になる。`alive` が「生きている Pod」を Pending + Running と数え、Failed を死んだものとして除くのが後で効く。この Cluster は、Kubernetes で言えば API サーバが保持する実際の状態にあたる。コントローラはこの状態を観測し、desired state と突き合わせる。ここでは観測を単純に「今ある Pod を数える」こととした。

## ② Reconcile: 差を数えて埋める

調整の本体だ。desired(あるべき数)と observed(生きている数)を見比べ、差を埋める操作を出す。足りなければ作り、多ければ消す:

<<< ../../orchestration/reconcile/reconcile.go#reconcile{go}

`Reconcile` を読むと、やっていることは驚くほど単純だ。まず Failed な Pod を掃除する。次に生きている数を数え、desired より少なければ差のぶん作り、多ければ余りを消す。同じなら何もしない。この単純さが力になる。まず冪等だ。目標に達していれば何度呼んでも何も起こさない。テストで、収束後の Reconcile が no-op であることを固定した。だから調整ループは安心して回し続けられる。無駄に作ったり消したりしない。

そして、この 1 つの関数が 3 つの仕事を兼ねる。空から 3 個作るのも(作成)、目標を 2 から 5 に変えて 3 個足すのも(スケール)、落ちた Pod を数え直して作り直すのも(自己修復)、すべて「生きている数と desired の差を埋める」という同じ処理だ。障害回復のための特別なコードはどこにもない。Failed が生きた数に数えられないので、自然に作り直しの対象になる。テストで、Pod を落とすと次の Reconcile が作り直すこと、目標数を変えるだけで過不足が埋まることを固定した。

## ③ level-triggered: なぜイベント駆動でないのか

ここが調整ループの設計の核心だ。素朴には、コントローラを「Pod が死んだ」というイベントに反応させたくなる(edge-triggered)。だが Kubernetes はそうしない。コントローラは、イベントを聞くのでなく、毎回まるごと現状を数え直して desired と比べる(level-triggered)。

なぜか。edge-triggered は取りこぼしに弱い。「Pod が死んだ」イベントを 1 回逃したら、その死は永遠に修復されない。ネットワークの瞬断、コントローラの再起動、イベントの順序入れ替わり。分散システムでイベントを完璧に届けるのは難しく、必ずどこかで取りこぼす。level-triggered なら、イベントを何回逃しても関係ない。次の調整で現状を数え直し、desired と違えば直す。「今どうであるか」だけを見て、「何が起きたか」に依存しない。テストで、3 つの Pod をまとめて落とし、その間 1 度も Reconcile しなくても、たった 1 回の Reconcile が全部を復旧することを固定した。イベントを 3 回逃したのと同じ状況で、それでも収束する。

この堅牢さが、Kubernetes が「宣言した状態にいつか必ず収束する(eventually consistent)」と言える理由だ。コントローラが一時的に落ちても、復帰して次に調整すれば追いつく。宣言的 API と level-triggered な調整ループ。この 2 つが組み合わさって、自己修復するシステムができる。

### 動かす

下のデモは、目標レプリカ数と実際の Pod を並べて、調整ループが差を埋める様子を見る。Pod をわざと落としたり、目標数を変えたりして「調整」を押すと、同じループが自己修復もスケールもこなす。イベントを溜めてから 1 回調整しても追いつく level-triggered の強さも確かめてほしい。

<ReconcileDemo />

## 設計の観点

- **宣言的 API が土台**: 手順でなく状態を宣言するから、コントローラは「今の状態から目標へ」を毎回計算できる。手続き的だと途中で失敗したとき、どこまで進んだかの復旧が難しい
- **level-triggered は分散の必然**: イベントは取りこぼす前提で設計する。現状を数え直す方式なら、取りこぼしも重複も順序入れ替わりも吸収する。Kubernetes 全体がこの原則で貫かれている
- **冪等な reconcile**: 何度呼んでも安全だから、調整ループは頻繁に・重複して回せる。冪等でないと、二重に作ったり消したりする
- **コントローラは無数にある**: ReplicaSet だけでなく、Deployment・Job・Service など、リソースごとに専用のコントローラが同じパターンで走る。Kubernetes は「調整ループの集合」だ
- **watch と informer**: 実物は毎回全件を数えるのは重いので、変更を watch してキャッシュ(informer)を保ち、変わったものだけ reconcile を呼ぶ。だが reconcile 自体は常に「現状 vs 目標」で、level-triggered の性質は保たれる

## 対照と実例

| | 手続き的 / edge-triggered | 宣言的 / level-triggered |
|---|---|---|
| 人が書くもの | 手順(作れ・消せ) | 状態(3個であれ) |
| 反応の起点 | イベント | 現状の数え直し |
| 取りこぼし | 修復されない | 次の調整で追いつく |
| 障害回復 | 専用の処理が要る | 同じループが処理 |
| 例 | 素朴なスクリプト | Kubernetes コントローラ |

裏どり:

- **Kubernetes: Controllers**: 調整ループの公式説明。desired/observed の突き合わせと収束の考え方
- **level-triggered vs edge-triggered (James Bowes 他)**: Kubernetes が level-triggered を選んだ設計判断の解説。分散環境での堅牢性
- **[Operator パターン](/parts/operator)**: 調整ループを自作のリソースに広げる仕組み。同じパターンでアプリ固有の運用を自動化する
- **Reconciler (controller-runtime)**: 実装フレームワーク。`Reconcile(req) (Result, error)` の 1 メソッドが本章の Reconcile にあたる

## 簡略化したこと

- **ReplicaSet 相当のみ**: Deployment の revision 管理は扱わない。入れ替えそのものは別章([ローリング更新](/parts/rollout))
- **単一コントローラ**: 実物は多数のコントローラが並行に走り、共有 API サーバの状態を watch する
- **watch でなく明示 Reconcile**: 実物は informer が変更を検知して reconcile を呼ぶ。ここは手で呼ぶ
- **スケジューリングなし**: Pod をどのノードに置くかは次章([スケジューラ](/parts/pod-scheduler))

## 参考資料

- [Kubernetes: Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) — 調整ループの公式解説
- [Level Triggering and Reconciliation in Kubernetes (Hausenblas/Schimanski)](https://cluster-api.sigs.k8s.io/) — level-triggered 設計の背景
- [Kubebuilder Book](https://book.kubebuilder.io/) — Reconciler の実装パターン
- 実装: [orchestration/reconcile](https://github.com/esh2n/sharin/tree/main/orchestration/reconcile)
