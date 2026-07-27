# congestion — TCP 輻輳制御

TCP の輻輳ウィンドウ制御(スロースタート + AIMD)を最小構成で実装する。中心は「AIMD が中央調整なしに効率と公平を両立する」ことを、のこぎり波と 2 接続の収束で示すこと。

## 肝

- **輻輳ウィンドウ(cwnd)**: 一度に送ってよい量。誰も全体の空き帯域を知らないので、各接続が自分で探る
- **スロースタート**: cwnd=1 から始め、1 往復ごとに倍にする。空き帯域を手早く探る。ssthresh に達したら輻輳回避へ
- **輻輳回避**: 1 往復に 1 だけ増やす(加算増加)。容量の近くを慎重に探る
- **損失で半減**: パケットが落ちたら「詰まった」合図。ssthresh と cwnd を半分に(乗算減少)。タイムアウトなら cwnd=1 に戻しスロースタート
- **AIMD**: 加算増加 + 乗算減少。増やしては切られるのこぎり波で容量を探り続ける
- **公平性**: 加算増加は接続間の差を保ち、乗算減少は差を半分にする。だから取り分が等しい方へ収束する

## 効果の固定(テスト)

- `TestSawtooth`: cwnd が容量を探って増減を繰り返す(のこぎり波)
- `TestAIMDConvergesToFairness`: 偏った初期(30 対 2)でも取り分が等しい方へ収束
- `TestSlowStartDoubles`: 倍々で増え、ssthresh で輻輳回避に切り替わる

## 使い方

```go
c := congestion.New(ssthresh)
c.OnRoundACKed() // 1 往復ぶんの ACK: 状態に応じて cwnd を増やす
c.OnLoss()       // 重複 ACK による損失: cwnd/ssthresh を半分に
c.OnTimeout()    // タイムアウト: cwnd=1、スロースタートから

hist := congestion.Simulate(ssthresh, capacity, rounds)   // のこぎり波
hA, hB := congestion.SimulateFairness(cap, a0, b0, rounds) // 2 接続の収束
```

## 簡略化したこと

- **TCP Reno 相当**: 実物は Cubic(既定)や BBR(帯域と RTT を測る)など多様。ここは古典的な AIMD
- **1 往復単位**: 個々の ACK でなく往復ごとにまとめて増やす。RTT やパケット単位は抽象化
- **損失は容量超過で判定**: 実物は重複 ACK やタイムアウトで検知。ここは cwnd>容量を損失とみなす
- **ネットワーク公平性の理想化**: 両接続が同時に損失を見る前提。実際はもっと複雑

## 章

教科書: [輻輳制御(AIMD)](https://sharin-2a1.pages.dev/parts/congestion)

実行: `go test ./network/congestion/`
