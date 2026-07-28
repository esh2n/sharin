<script setup>
import PodSecurityDemo from '../components/PodSecurityDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# SecurityContextとPod Security Standards

> 実装: [`orchestration/podsecurity/`](https://github.com/esh2n/sharin/tree/main/orchestration/podsecurity) / 実行: `go test ./orchestration/podsecurity/`

<Summary>
RBAC は誰が操作してよいかを決め、admission は何を受け入れるかを決めた。残るのは、受け入れた Pod がノードの上でどこまでできるかになる。ここは書けば効くが、書かなければ何も制限しない。だから名前空間から一律に決める層が要る。そして同じ検査を、拒否・記録・警告の3通りに使い分けられる。この分離が、動いているものを止めずに締める方法になる。
</Summary>

## この章で作るもの

[RBAC](/parts/rbac) は「誰が操作してよいか」を決めた。[admission webhook](/parts/admission) は「どんな内容なら受け入れるか」を決めた。だが、受け入れた後がまだ残っている。その Pod は、ノードの上でどこまでできるのか。

[コンテナ](/parts/container)の章で見たとおり、コンテナは名前空間と cgroup で隔離されている。だがその隔離は、いくらでも緩められる。ホストのネットワークをそのまま使う、ホストのディレクトリを覗く、特権つきで動かす。どれも設定ひとつで外せる。

外せることには理由がある。ログを集める常駐はホストのディレクトリを読まなければならないし、ノードの指標を取る常駐はホストのネットワークを見なければならない。[DaemonSet](/parts/daemonset) の章で扱ったような仕事は、隔離を外さないと成り立たない。

問題は既定にある。`SecurityContext` は書けば効くが、書かなければ何も制限しない。[ResourceQuota と LimitRange](/parts/quota) の章で見た「既定値が無いと書き忘れが 0 として通る」のと同じ形で、権限のことを何も書かなかった Pod がいちばん緩い設定で動く。書き忘れがいちばん危険な状態になる。

だから、名前空間の側から一律に決める層が要る。それが Pod Security Standards になる。

<FigureBox caption="baseline は「危ないことをしていないか」、restricted は「安全を書いたか」。向きが違う">

```
                    privileged      baseline       restricted
                    何も見ない      隔離を外して    安全側を
                                    いないか        明示したか

  何も書かない Pod     通る           通る          落ちる
                                                    ↑ 4項目を書き足せば通る

  隔離を外した Pod     通る          落ちる         落ちる
  (ログ収集など)                    ↑ hostPath / hostNetwork /
                                       privileged / capabilities

  書き足した Pod       通る           通る           通る


  同じ検査を3つの扱いで使い分ける

  ┌── Check(pod, enforce) ──→ 違反あり → 拒む
  ├── Check(pod, audit)   ──→ 違反あり → 記録に残す(通す)
  └── Check(pod, warn)    ──→ 違反あり → 作った人に伝える(通す)

  enforce=baseline / warn=restricted にしておくと、
  今動いているものを止めずに、何を直せばよいかだけが伝わる
```

</FigureBox>

肝は3つ:

1. **既定は許可**: 書かなければ制限されない。だから外から一律に決める層が別に要る
2. **段階で向きが変わる**: baseline は「していないか」、restricted は「書いたか」を見る
3. **判定は1つ、扱いが3つ**: 拒否・記録・警告。この分離が、締める操作を安全にする

## ① 書かなければ制限されない

まず、権限の設定を型にする:

<<< ../../orchestration/podsecurity/podsecurity.go#pod{go}

`AllowPrivilegeEscalation` などがポインタになっているのが、この章でいちばん地味で大事なところになる。「書かなかった」と「false と書いた」を区別する必要があるからだ。

区別が要るのは、書かなかったことが違反になる規則があるためになる。ただの `bool` にしてしまうと、書かなかった Pod は `false` として届き、検査を通ってしまう。それでは「安全側を明示したか」を確かめられない。テストで、書かなかった場合と `true` と書いた場合が両方とも違反になり、`false` と書いた場合だけが通ることを固定した。

そして、ここまでの設定は Pod を作る人が書くものになる。書かなければ何も制限されないので、書き忘れた Pod は特権つきでこそないものの、root で動き、権限を落とさず、昇格もできる状態になる。

## ② 段階で向きが変わる

検査はこうなる:

<<< ../../orchestration/podsecurity/podsecurity.go#check{go}

`checkBaseline` が見ているのは、どれも「ホストとの隔離を外していないか」になる。ホストのネットワーク、ホストのプロセス、ホストのディレクトリ、特権、権限の追加。外せばコンテナの中から外に手が届くので、そこを塞ぐ。

`checkRestricted` は向きが逆になる。危ないことをしていないかではなく、安全側を明示したかを見る。`allowPrivilegeEscalation: false` と書いたか、`runAsNonRoot: true` と書いたか、権限を `ALL` 外したか、seccomp を指定したか。

この向きの違いが、実際の効き方の違いになる。テストで、権限のことを何も書いていない素朴な Pod が baseline は通り、restricted では4つの違反で落ちることを固定した。何も悪いことをしていないのに落ちるのは、何も書いていないからだ。

段階が積み重なっていることも固定した。restricted の検査は baseline の検査を含む。上の段階だけを別に書いてしまうと、下の規則が抜けても誰も気づけない。

## ③ 判定は1つ、扱いが3つ

締める操作は、そのままでは危ない。名前空間を restricted にした瞬間、そこにある Pod のほとんどが作り直せなくなる。だが、作り直そうとするまで誰も気づかない。次のデプロイで初めて落ちる。

だから3つの扱いが用意されている:

<<< ../../orchestration/podsecurity/podsecurity.go#policy{go}

`Admit` は同じ `Check` を3回呼ぶだけで、渡す段階だけが違う。判定は1つで、扱いが3つになっている。[ヘルスチェック](/parts/probe)の章で「検査は同じ、失敗したときの扱いだけが違う」と書いたのと、まったく同じ構造になる。

使い方はこうなる。拒否を baseline に、警告を restricted にしておく。今動いているものは baseline を通るので止まらない。だが restricted に届いていない Pod を作ろうとすると、通りはするが「ここが足りない」と伝わる。直った頃に拒否を restricted へ上げる。

`Tighten` はその判断を先に行うためのものになる。今ある Pod を全部通したまま段階を上げられるか、上げるなら何が落ちるかを、実際に落とさずに返す。テストで、baseline へ上げると隔離を外している1つだけが落ち、restricted へ上げると2つ落ちること、直したものだけなら何も落ちないことを固定した。

締める前に何が壊れるかが分かる、という性質が無ければ、この手の設定は誰も上げなくなる。3つの扱いは、そのために分かれている。

### 動かす

下のデモは、3つの Pod を3つの段階に照らす。段階を選ぶと、通るものと落ちるものが入れ替わる。拒否と警告を別々に設定すると、通ったうえで何が足りないかだけが出る状態を作れる。

<PodSecurityDemo />

## 設計の観点

- **既定が許可なら、外側に層を足すしかない**: 個々の設定に任せると、書き忘れがいちばん緩くなる
- **未設定と false を区別する**: 明示を求める規則は、区別できなければ検査できない
- **段階は含む形で作る**: 上だけを別に書くと、下の規則の抜けに気づけない
- **判定と扱いを分ける**: 同じ検査を、拒否・記録・警告に使い分けられると、締める操作が安全になる
- **例外は名前空間で切る**: 隔離を外す必要がある常駐は、専用の名前空間に置いて、そこだけ緩める
- **[RBAC](/parts/rbac)との役割分担**: 誰が(RBAC)、何を(admission)、どこまで(これ)。3つとも要る

## 対照と実例

| | RBAC | admission webhook | Pod Security Standards |
|---|---|---|---|
| 決めること | 誰が操作してよいか | どんな内容を受け入れるか | 受け入れた後どこまでできるか |
| 効くところ | API を叩く時点 | 保存される直前 | ノードで動くとき |
| 既定 | 拒否 | 何も無ければ通す | 許可(privileged) |
| 単位 | 主体と資源の種類 | 資源の中身 | 名前空間 |
| 段階的な導入 | 役割を足す | failurePolicy を Ignore に | audit と warn を先に上げる |

裏どり:

- **3つの段階**: `privileged`(制限なし)、`baseline`(既知の権限昇格を塞ぐ)、`restricted`(現在の強化指針まで)
- **名前空間ラベル**: `pod-security.kubernetes.io/enforce`、`audit`、`warn` の3つを独立に指定する。値は段階名
- **バージョン指定**: `enforce-version` などで、どの版の基準を使うかを固定できる。基準そのものが版で変わるため
- **PodSecurityPolicy は削除済み**: 1.25 で削除され、Pod Security Admission が後継になった
- **restricted が求める4項目**: `allowPrivilegeEscalation: false`、`runAsNonRoot: true`、`capabilities.drop: [ALL]`、`seccompProfile` の指定
- **さらに細かく決めたい場合**: Kyverno や Gatekeeper のような[admission webhook](/parts/admission)を足す。組み込みの段階は3つしかない

## 簡略化したこと

- **規則が一部だけ**: 実物の baseline / restricted はもっと多くの項目を見る。ここは代表的なものだけ
- **ラベルの解釈なし**: 名前空間オブジェクトからラベルを読む部分は扱わない。`Policy` を直接渡す
- **版の固定なし**: `enforce-version` に相当するものは持たない
- **例外なし**: 特定の ServiceAccount や利用者を検査から外す仕組みは扱わない
- **実行時の強制なし**: ここでは受け入れの判定だけ。実際に権限を落とすのは kubelet とランタイムの仕事
- **ボリュームの種別なし**: `hostPath` 以外のボリューム制限は扱わない

## 参考資料

- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/) — 3つの段階と、各段階が見る項目
- [Pod Security Admission](https://kubernetes.io/docs/concepts/security/pod-security-admission/) — 名前空間ラベルと enforce / audit / warn
- 実装: [orchestration/podsecurity](https://github.com/esh2n/sharin/tree/main/orchestration/podsecurity)
