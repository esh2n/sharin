# eventsourcing — イベントソーシング

状態でなく「起きた出来事の連なり」を真実の源にする。現在状態はイベントの畳み込み(リプレイ)で導き、履歴が丸ごと残る。中心は「状態は保存せず導出する」を口座の残高で示すこと。

## 肝

- **イベントが真実**: 保存するのは追記専用のイベント列(「1000入金」「300出金」…)。上書き・削除はしない
- **状態は導出**: 現在残高はイベントを頭から畳み込めばいつでも作れる。状態自体は保存しない射影
- **全履歴が残る**: いつ・なぜその状態になったかが残る。監査ログそのもの
- **タイムトラベル**: version を指定すれば過去のある時点の状態に遡れる
- **コマンドとイベントは別**: コマンド(意図)は検証を通ってはじめてイベント(事実)になる。残高超過の出金は拒否し、イベントを残さない
- **スナップショット**: 途中の状態を写し取り、それ以降のイベントだけ畳んで速くする

## 効果の固定(テスト)

- `TestReplayDerivesState`: イベントを畳んで残高 1200 を導出
- `TestTimeTravel`: version ごとに過去の残高(0/1000/1500/700)を再現
- `TestCommandValidation`: 残高超過の出金は ErrInsufficient で、イベントを残さない
- `TestSnapshotMatchesFullReplay`: スナップショット + 以降のイベント = 全リプレイ

## 使い方

```go
var s eventsourcing.Store
eventsourcing.Deposit(&s, 1000)
eventsourcing.Withdraw(&s, 300)      // 残高を検証してから追記
now := eventsourcing.Replay(s.Events())          // 現在状態を導出
past := eventsourcing.StateAt(s.Events(), 1)     // 過去の時点へ
snap := eventsourcing.TakeSnapshot(now)
cur := eventsourcing.RestoreFrom(snap, s.EventsAfter(snap.Version))
```

## 簡略化したこと

- **単一集約**: 口座 1 つ分。実物は集約 ID ごとにストリームを分ける
- **メモリ内ストア**: 追記専用ログを配列で保持。実物は永続化(追記専用DB/Kafka 等)
- **同期・単一スレッド**: 並行追記の楽観ロック(バージョン競合)は扱わない
- **イベントの版管理なし**: スキーマ変更(アップキャスト)は省略

## 章

教科書: [イベントソーシング](https://sharin-2a1.pages.dev/parts/event-sourcing)

実行: `go test ./messaging/eventsourcing/`
