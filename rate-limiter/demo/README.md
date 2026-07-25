# rate-limiter/demo

教科書の章から実際に叩けるライブデモ。Cloudflare Workers + Durable Objects 上で
Go 版と同じ4方式(token bucket / leaky bucket / fixed window / sliding window log)を
`GET /check?algo=<方式>` で提供する。bucket 系は容量5・0.5個/秒、window 系は
limit 5・窓10秒、いずれも「方式 + IP」ごとに判定。

## 肝

- 分散環境の課題は「残量の read-modify-write をどう原子的にするか」に尽きる
- Durable Object は「キーごとに世界で1つのインスタンス」なので、そこに状態を置けば
  リクエストが勝手に直列化され、原子性がタダで手に入る(Redis + Lua の代替解)
- 判定計算そのものは Go 版と同じロジック。純粋関数(algorithms.ts)に切り出してテストしている

## 簡略化したこと

- キーは IP のみ(実務なら API キーやユーザーIDと組み合わせる)
- Durable Object が単一点になる遅延・費用の議論は省略
- デモ用に固定パラメータ(容量5、補充0.5個/秒)をハードコード

## 操作

```sh
pnpm test        # バケツ計算のユニットテスト
pnpm typecheck
pnpm dev         # ローカル実行
pnpm deploy      # デプロイ(要 wrangler ログイン)
```
