# service — 変わらない宛先と、各ノードに配られるルール

Pod は消えては生まれ、IP が変わる。Service は変わらない仮想 IP を1つ用意する。だがその IP に対応する実体はどこにもない。あるのは各ノードに配られた書き換えルールだけ。

## 肝

- **仮想 IP に実体はない**: ClusterIP で待ち受けている者は誰もいない。パケットが出ていく瞬間に、そのノードのルールで宛先が書き換わるだけ
- **振り分けはノードの中で完結する**: 中央に集約点がないので、そこが混んだり落ちたりしない。順番を数えるカウンタもノードごとに独立
- **セレクタで集める**: ラベルが合い、かつ ready な Pod だけが宛先になる。名前を1つずつ登録するのではない
- **ルールの配布には遅れがある**: 制御側が決めた瞬間には変わらない。この遅れが[終了処理](../lifecycle/)の事故の正体
- **ルールは Service と宛先の積**: この増え方が iptables 方式の重さになる。IPVS 方式はここを表引きに置き換える
- **決定性**: 振り分けはノードごとの順番で決める。実物は確率だが、ここは再現性を優先

## 効果の固定(テスト)

- `TestVirtualIPHasNoBackingPod`: ClusterIP を持つ Pod は存在せず、あるのは各ノードのルールだけ
- `TestEachNodeRoutesIndependently`: ノードごとに順番を持つので、選び方がずれる
- `TestStaleRulesSendToDeadPod`: 配り終わる前は、消えた Pod の IP へパケットが飛ぶ
- `TestUnreadyPodLeavesRules`: ready を落とすだけで宛先から外れ、戻せば復帰する
- `TestNoEndpointsDropsPacket`: 宛先が無くなるとルール自体が消え、行き場を失う
- `TestRuleCountGrowsWithServicesAndPods`: 3 Service × 4 宛先で 12 本

## 使い方

```go
c := service.New(service.Config{Propagation: 3})
na := c.AddNode("node-a")
c.AddNode("node-b")
c.AddService("web", "10.0.0.1", map[string]string{"app": "web"})
c.AddPod("web-1", "10.1.0.1", map[string]string{"app": "web"}, true)
c.AddPod("web-2", "10.1.0.2", map[string]string{"app": "web"}, true)

for i := 0; i < 4; i++ { c.Tick() } // ルールが配り終わるのを待つ
c.Send(na, "10.0.0.1")              // node-a から仮想 IP 宛にパケットを1つ

c.Endpoints("web")   // 制御側から見たあるべき宛先
na.Rules("10.0.0.1") // node-a が今持っているルール(ずれることがある)
c.Converged()        // 両者が一致しているか
```

## 簡略化したこと

- **ポート番号なし**: 実物は Service のポートと Pod のポートを対応づける。ここは宛先 IP だけ
- **振り分けは順番**: 実物の iptables 方式は確率で選ぶ。決定性を優先して順番にした
- **セッションアフィニティなし**: 同じ相手を同じ Pod へ固定する設定は扱わない
- **ClusterIP のみ**: NodePort / LoadBalancer / Headless は扱わない
- **論理時刻**: 実時間でなく周期で数える

## 章

教科書: [Serviceとkube-proxy](https://sharin-2a1.pages.dev/parts/service)

実行: `go test ./orchestration/service/`
