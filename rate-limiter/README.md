# rate-limiter

## 肝

- レートリミットとは「バーストをどこまで許すか」と「平滑化」のトレードオフの選択である
- タイマーは要らない。呼ばれた瞬間に経過時間から補充/漏れを計算すればよい(lazy refill)
- fixed window は境界をまたぐと limit の2倍が通る。この弱点はテストで固定してある

## 簡略化したこと

- 単一プロセス・単一キーのみ(分散やキーごとの制限は扱わない)
- `Allow` の二値判定のみで、待機(`Wait`)や複数トークン消費はない
- sliding window log の記録は素朴なスライス管理

本文: [教科書の章](../docs/parts/rate-limiter.md) / 実行: `go test ./rate-limiter/`
