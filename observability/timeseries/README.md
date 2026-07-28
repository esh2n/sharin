# timeseries — 整列と集約、そして誤読

[metrics](../metrics/) は1台のプロセスが値をどう持つかを見た。画面に線が出るまでには、
もう2段ある。同じメトリクスは Pod やインスタンスの数だけ別々の系列として届き、
それぞれが不揃いな時刻に点を打っている。1本の線にするには、まず時間方向に揃え(整列)、
次に系列方向にまとめる(集約)。

どちらも「たくさんの数を1つにする」操作なので混同されやすいが、順序が決まっていて、
順序が結果を変える。

## 再現する誤読

| 誤読 | 何が起きるか | 直し方 |
|---|---|---|
| 累計をそのまま描く | 増える一方の線しか見えない。再起動で負に落ちる | `AlignDelta` / `AlignRate` |
| 各系列の p99 を平均する | 遅い1台が速い多数に薄められる | 分布のまま `ReduceP99` |
| 平均で見る | 刺さっている1台が消える | `ReduceMax` を併せて見る |
| 窓を広げる | 短いスパイクが平滑化されて消える | 窓を狭めるか `AlignMax` |

すべて固定テストにしてある。

## 使い方

```go
// ① 整列: 系列ごとに、時間方向で窓にまとめる
aligned := make([]timeseries.Series, 0, len(raw))
for _, s := range raw {
    aligned = append(aligned, timeseries.Align(s, timeseries.AlignDelta, 60))
}

// ② 集約: 残すラベルを決めて、系列方向でまとめる
byZone := timeseries.Reduce(aligned, timeseries.ReduceP99, "zone")
```

分布値の系列に `AlignDelta` を使うと、窓の中のヒストグラムを足し合わせた分布が出る。
分布のまま残るのが大事なところで、ここで分位点にしてしまうと後で正しく足せなくなる。

## 種類

| Kind | 意味 | そのまま読めるか |
|---|---|---|
| `Gauge` | その瞬間の値 | 読める |
| `Delta` | 前回からの増分 | 区間の合計に意味がある |
| `Cumulative` | 計測開始からの累計 | 差を取らないと読めない |

`Cumulative` の差分では、前の窓より小さくなったら数え直し(再起動)とみなす。
この検出を忘れると、再起動のたびに大きな負の値が出る。

## API

| 関数 | 役割 |
|---|---|
| `Align(s, aligner, period) Series` | 1本の系列を等間隔の窓に揃える |
| `Reduce(all, reducer, groupBy...) []Series` | 残すラベルを決めて系列をまとめる |
| `Series.Distribution() bool` | 分布値を持つ系列か |

`Aligner` は `AlignNone` / `Mean` / `Max` / `Min` / `Sum` / `Delta` / `Rate` / `P50` / `P99`。
`Reducer` は `ReduceNone` / `Mean` / `Sum` / `Max` / `Min` / `P50` / `P99`。
`String()` は Cloud Monitoring の enum 名(`ALIGN_PERCENTILE_99` など)を返す。

## 決定性

乱数も実時間も使わない。窓の割り当ては `t / period` だけで決まり、
まとめる順序はラベルの並べ替えで固定される。同じ入力なら必ず同じ結果になる。

ヒストグラムは [metrics](../metrics/) パッケージの `Histogram` をそのまま使う。
足せることがヒストグラムの取り柄で、その足し算があるから「まとめてから分位点」が成り立つ。

## テスト

```
go test -race -cover ./observability/timeseries/
```

カバレッジ 100%。
