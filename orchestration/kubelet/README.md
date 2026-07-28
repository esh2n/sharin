# kubelet — 決まった配置を、実際に動かす

[scheduler](../scheduler/) は Pod をどのノードに置くかを決めた。だが決めただけでは
何も動かない。決まった配置を実際にプロセスとして起こすのが、各ノードで動いている kubelet になる。
この編の、いちばん下の端にあたる。

## 今までのコントローラとの違い

kubelet も調整ループになっている。あるべき姿と現状を比べて差を埋める。違うのは1点だけで、
**現状の取得先が置き場ではなくランタイム**になる。「宣言ではこうなっている」と
「実際にプロセスが動いている」を突き合わせるのは、この層が初めてになる。

聞き方も面白い。ランタイムは変化を教えてくれる仕組みを持っているが、kubelet はそれに頼らず
定期的に一覧を取り直す。[reconcile](../reconcile/) の level-triggered が、いちばん下の層でも
同じ形で現れている。

```go
rt := kubelet.NewRuntime()
k := kubelet.New(rt)

k.SetFilePods([]kubelet.PodSpec{apiserverPod}) // ノード上のファイル
k.Link(true)
k.Deliver([]kubelet.PodSpec{webPod})           // 置き場から届いた

for i := 0; i < 5; i++ {
    k.Tick() // 世界が進む → 一覧を取り直す → 差を埋める
}
k.Running()  // ["kube-apiserver/apiserver", "web/app"]
rt.Relists   // 5 ← 毎周取り直している
```

## 2つの出所

| Source | どこから来るか | 置き場を見失ったら |
|---|---|---|
| `FromAPIServer` | 置き場から届く | 最後に届いた内容を守り続ける |
| `FromFile` | ノード上のファイル | 何も変わらない |

ファイルから読む Pod が、鶏と卵を解く。置き場そのものが Pod として動いているとき、
その置き場を誰が起こすのか。kubelet が、置き場を経由せずに起こす。

```go
k := kubelet.New(rt)          // まだ置き場に繋がっていない
k.SetFilePods([]kubelet.PodSpec{apiserverPod})
run(k, 4)                     // → kube-apiserver が動き出す
k.Link(true)                  // 置き場が立ったので繋がる
k.Deliver([]kubelet.PodSpec{webPod})
```

## CRI という境界

`Runtime` が CRI の向こう側になる。kubelet が知っているのは「作れ」「消せ」「一覧をくれ」の
3つだけで、その先が何であるかは知らない。境界を1枚置いたことが、実装を差し替えられることの正体になる。

| メソッド | 役割 |
|---|---|
| `Create(pod, spec) string` | コンテナを作り、識別子を返す |
| `Remove(id)` | 消す |
| `List() []ContainerStatus` | 一覧を取り直す(`Relists` が増える) |
| `Step()` | 実際の世界が1つ進む。kubelet は呼ばない |

## API

| 関数・メソッド | 役割 |
|---|---|
| `New(rt) *Kubelet` | ランタイムに繋がった kubelet |
| `SetFilePods(ps)` / `Deliver(ps) bool` | 2つの出所からの宣言 |
| `Link(up)` / `Linked() bool` | 置き場との接続 |
| `Sync()` | 一覧を取り直して差を埋める |
| `Tick()` | 世界を進めてから調整する |
| `Running() []string` / `Restarts(pod, name) int` | 観測 |

## 決定性

実時間も乱数も使わない。起動にかかる時間と落ちるまでの時間は `ContainerSpec` に台本として与える。
一覧は Pod 名、コンテナ名の順。作り直しの待ち時間は 1, 2, 4, 8 と倍に伸びて上限で止まる。

## テスト

```
go test -race -cover ./orchestration/kubelet/
```

カバレッジ 100%。
