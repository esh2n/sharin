<script setup>
import RateLimitDemo from '../components/RateLimitDemo.vue'
</script>

# Rate Limiter

> 実装: [`rate-limiter/`](https://github.com/esh2n/sharin/tree/main/rate-limiter) / 実行: `go test ./rate-limiter/`

## この章で作るもの

代表的なレートリミットアルゴリズム4方式を、同じインターフェースで実装して比較する。

1. **Token Bucket** — バーストを許しつつ平均レートを守る
2. **Leaky Bucket** — 流出速度を一定に保つ
3. **Fixed Window** — 実装は最も単純、ただし境界に弱点がある
4. **Sliding Window Log** — 正確、ただしメモリを食う

各方式の説明の下には**ライブデモ**を置いてある。Cloudflare Workers 上で動いている
本物のレートリミッター([実装](https://github.com/esh2n/sharin/tree/main/rate-limiter/demo))を
その場で叩いて、説明した挙動をすぐ確かめられる。判定はあなたの IP ごとなので、遠慮なく連打してほしい。
デモで返ってくる **429** は "Too Many Requests"(リクエストが多すぎる)を表す
HTTP ステータスコードで、レートリミッターが拒否したときの標準の返事。**200** は成功。

この章の肝は3つ。

- レートリミットとは「**バーストをどこまで許すか**」と「**平滑化**」のトレードオフの選択である
- タイマーは要らない。**呼ばれた瞬間に経過時間から計算すればいい**(lazy refill)
- fixed window は境界をまたぐと limit の**2倍**が通る。この弱点はテストで固定してある

## なぜレートリミットが必要か

サーバーの処理能力は有限で、クライアントの要求速度は制御できない。この非対称を放置すると、
悪意の有無に関係なく(バグったリトライループ1つで)サービス全体が落ちる。

レートリミッターは両者の間に立って「単位時間あたり何件まで通すか」を強制する部品で、
API ゲートウェイ、ログインの試行制限、スクレイピング対策、LLM API のトークン制限など、
外に開いた口のほぼすべてに入っている。

設計上の本質的な問いは1つだけ:

> 「平均 10 req/s まで」と言ったとき、**一瞬に10発**は許すのか、**100msに1発ずつ**しか許さないのか。

前者に寄せたのが token bucket、後者に寄せたのが leaky bucket、
「そもそも平均をどう測るか」を単純化したのが window 系、と整理できる。

どちらが正しいかは決まっていない。**誰を守りたいか**で決まる。
ユーザー体験を守りたい(画面を開いた瞬間に飛ぶ10本のAPIやリトライの重なりを弾きたくない)なら
バーストは許したいし、下流のDBや外部APIの瞬間処理能力を守りたいならバーストこそが敵になる。
以降の各方式には、この観点でのメリット/デメリットと実プロダクトでの採用例を添える。

## 共通インターフェース

4方式とも「今この1リクエストを通すか」だけを答える。

<<< ../../rate-limiter/limiter.go#limiter{go}

テストでは時計を注入して(`now func() time.Time`)、時間を自由に進めながら挙動を検証する。
時刻を外から与えられるようにするのは、時間が絡むコードをテスト可能にする定石。

## 1. Token Bucket

バケツにトークンが一定速度で補充され、リクエストは1トークン消費できたときだけ通る。
バケツの容量ぶんだけ「溜め」ができるので、しばらく静かだったクライアントの瞬間的なバーストは
**容量までは素通しする**。これは特徴であって、無条件の長所ではない。

<<< ../../rate-limiter/token_bucket.go#tokenbucket{go}

実装の見どころは `Allow` の冒頭。トークンをタイマーで定期的に足すのではなく、
**前回呼ばれてからの経過時間 × 補充速度**をその場で計算して足している(lazy refill)。
これで goroutine もタイマーも不要になり、状態は「残量」と「最終計算時刻」の2つで済む。

```
時刻:     0s          1s          2s
補充:     満タン(3)    +1/s で回復
リクエスト: ●●●○        ●○
           ↑3発は通る   ↑1秒で1トークン回復したので1発だけ通る
           ↑4発目は空
```

**メリット**

- 正当なバーストを弾かない。「平均的には行儀がいいが瞬間的には固まる」クライアント
  (ページ初期表示の一斉API呼び出し、リトライの重なり)と相性がいい
- バースト許容量(=容量)と平均レートを**別々のつまみ**として設計できる
- 状態が残量と時刻の2つだけで軽い

**デメリット**

- 瞬間負荷は容量ぶん素通しする。下流のDBや外部APIが瞬間負荷に弱いなら、
  それは守れていない。「平均は守るが瞬間は守らない」方式だと理解して容量を決める必要がある
- 多数のクライアントのバケツが同時に満タンだと、理論上は全員分の容量が一斉に着弾しうる

**実例**

- AWS EC2 API のスロットリング(公式ドキュメントが token bucket 方式と明言している)
- Stripe API(採用理由を[公式ブログ](https://stripe.com/blog/rate-limiters)で解説)
- Go 準公式の `golang.org/x/time/rate`、Java の Guava `RateLimiter`

**試してみる**(容量5、補充0.5個/秒): 連打すると6発目で 429 になり、
2秒待つごとに1発ぶん回復する。

<RateLimitDemo algo="token-bucket" />

## 2. Leaky Bucket

こちらは「水の入ったバケツの底に穴が空いている」モデル。リクエストは水1杯ぶんで、
溢れなければ受け付ける。水は一定速度で漏れていく。

<<< ../../rate-limiter/leaky_bucket.go#leakybucket{go}

コードを見比べると token bucket とほぼ対称なことが分かる。
`tokens`(残り許可)を減らしていくか、`water`(使用量)を増やしていくかの違いで、
数学的には同じ制約を課している。区別する意味があるのは、leaky bucket を
**キュー**として実装した場合(溢れたら捨てるのではなく並ばせ、一定速度で処理する)。
その形は流量が完全に平滑化される代わりに遅延が生まれる。トラフィックシェーピングの文脈で
leaky bucket と呼ばれるのは主にこちら。

**メリット**

- キュー形にすれば下流に流れる速度が**完全に一定**になる。下流の処理能力を守ることが
  最優先の場面(容量の決まったワーカー、脆い外部API)に最適

**デメリット**

- 正当なバーストも均される。即時拒否の形はユーザー体験が悪くなりがちで、
  キュー形は遅延が積み上がる(待たされる)
- 「守りたいのは誰か」が下流に固定されている。ユーザー体験側を守る用途には向かない

**実例**

- NGINX の `limit_req`(公式ドキュメントが leaky bucket 方式と明言。burst パラメータはキュー長)
- Shopify Admin API(残量がレスポンスヘッダで返る leaky bucket)
- 通信機器のトラフィックシェーピング(この方式の出身地)

**試してみる**(容量5、漏れ0.5個/秒): 上の token bucket とまったく同じ結果になるはず。
双対であることを体感できる。

<RateLimitDemo algo="leaky-bucket" />

## 3. Fixed Window

時間軸を1秒なら1秒で固定に区切り、各区間のカウントが limit を超えたら拒否する。
カウンタ1個で済むので実装も分散化(Redis の `INCR` + `EXPIRE`)も一番簡単。

<<< ../../rate-limiter/fixed_window.go#fixedwindow{go}

### 境界バースト問題

fixed window には有名な弱点がある。**区間の境界をまたぐと、窓幅より短い時間に limit の2倍が通る**。

```
limit = 3/秒 のとき:

窓A [0.0s ────────── 1.0s) 窓B [1.0s ────────── 2.0s)
              ●●●          ●●●
             t=0.70       t=1.05

→ 0.35秒間に6リクエスト = 実質 17 req/s が通過
```

窓Aの終わり際に3発、窓Bが始まった直後に3発。どちらの窓も制限は守っているのに、
「直近1秒」で見ると2倍通っている。この挙動は
[`fixed_window_test.go`](https://github.com/esh2n/sharin/blob/main/rate-limiter/fixed_window_test.go)
に「仕様上の弱点」としてテストで固定してある。弱点を文章でなくテストで残しておくと、
後から読んだときに挙動として再現・確認できる。

**メリット**

- カウンタ1個と窓の開始時刻だけ。実装もメモリも最小
- 分散化が一撃(Redis の `INCR` + `EXPIRE` だけで動く)
- 「今月のAPI呼び出し回数」のような、窓自体に意味がある課金・クォータ用途では
  境界リセットがむしろ仕様として自然

**デメリット**

- 境界バーストで実質2倍通る。これを許容できるかが採用判断のすべて
- 窓リセットの瞬間に待機していたクライアントが一斉に再突入しやすい(サンダリングハード)

**実例**

- GitHub REST API(1時間窓で5000回。`X-RateLimit-Reset` が返すのは窓の終了時刻)
- 旧 Twitter API の15分窓
- 月次クォータをもつ SaaS の課金プラン全般

**試してみる**(limit 5、窓10秒): 5発使い切ると 429 になるが、「次に通るまで」は
窓の残り時間を指している。それだけ待つと窓が切り替わり、**一気にまた5発通る**。
境界バーストを自分の手で再現できる。

<RateLimitDemo algo="fixed-window" />

## 4. Sliding Window Log

境界バーストの根本原因は「窓が固定されていて、判定時刻を中心に見ていない」こと。
なら通したリクエストの時刻を全部記録して、毎回「**直近 window の間に何件通したか**」を
正確に数えればいい。それが sliding window log。

<<< ../../rate-limiter/sliding_window_log.go#slidingwindowlog{go}

fixed window で通ってしまった境界バーストのシナリオが、こちらでは正しく拒否される
(テストで対比している)。代償はメモリで、キーごとに最大 limit 件の時刻を保持する。
limit が大きい・キーが多い環境では効いてくる。

**試してみる**(limit 5、窓10秒): fixed window と同じ操作をしても、こちらは
「直近10秒に5件」が常に守られる。回復も窓の切り替わりで一気にではなく、
古い記録が1件ずつ窓から抜けるたびに1発ずつ戻る。

<RateLimitDemo algo="sliding-window-log" />

**メリット**

- 「直近N秒にM件まで」を**厳密に**保証する。4方式で唯一、判定が常に正確

**デメリット**

- キーごとに最大 limit 件の時刻を保持する。limit が大きい・キーが多いと効いてくる
- 分散化するとログの共有と掃除が必要で、他方式より重い

**実例**

- Redis の sorted set(`ZADD` + `ZREMRANGEBYSCORE` + `ZCARD`)で組む定番パターン
- ログイン試行制限や不正検知など、「厳密さがお金や安全に直結する」場面

なお中間案として、前の窓のカウントを経過割合で按分して足す **sliding window counter**
(Cloudflare 方式)があり、メモリはカウンタ2個のまま境界バーストをほぼ抑えられる。
近似で十分なら実務ではこれが有力。

## 4方式の比較

| 方式 | 状態 | バースト | 守るもの | 実例 |
|---|---|---|---|---|
| Token Bucket | 残量 + 時刻 | 容量まで許す | 平均レート(体験寄り) | AWS EC2 API、Stripe、`x/time/rate` |
| Leaky Bucket | 水位 + 時刻 | 均す | 下流の瞬間処理能力 | NGINX `limit_req`、Shopify API |
| Fixed Window | カウンタ + 窓開始 | 境界で limit の2倍 | 窓ごとの総量(クォータ) | GitHub REST API、月次課金プラン |
| Sliding Window Log | 時刻ログ | 正確に limit 以内 | 厳密さそのもの | Redis sorted set、ログイン試行制限 |

## 発展: 分散環境では

この章の実装は単一プロセスの mutex で守っているが、サーバーが複数台になると
「残量」をどこに置くかという問題になる。定石は Redis に状態を置き、
**読み取り、計算、書き込みを Lua スクリプトで原子的に実行する**こと。
アルゴリズム自体はこの章と同じで、変わるのは原子性の担保方法だけ。
これは db 編・proxy 編をやった後に戻ってくると解像度が上がるテーマ。

### デモの仕組み: 状態はどこにあるのか

実は各節のライブデモは分散版の実装で、残量は Redis ではなく Cloudflare の
**Durable Object** に置いてある。Durable Object は「キーごとに世界で1つだけ存在する
インスタンス」で、同じキー(このデモでは方式 + IP)へのリクエストは世界中どの経路から
来ても同じインスタンスに集められる。つまり read-modify-write が**勝手に直列化される**
ので、mutex も Lua スクリプトも書かずに原子性が手に入る。上の「分散環境では原子性の
担保方法が変わる」の、これが実答の1つ。

計算自体は Go 版と同じロジックを純粋関数
([algorithms.ts](https://github.com/esh2n/sharin/blob/main/rate-limiter/demo/src/algorithms.ts))
に切り出してあり、「状態をどこに置くか」と「どう計算するか」が分離されている。

curl でも同じエンドポイントを叩ける。`algo` には
`token-bucket` / `leaky-bucket` / `fixed-window` / `sliding-window-log` を指定できる:

```sh
for i in $(seq 6); do
  curl -s -o /dev/null -w '%{http_code} ' \
    "https://sharin-ratelimit-demo.esh2n.workers.dev/check?algo=fixed-window"
done
# 200 200 200 200 200 429
```

## 簡略化したこと

- **単一プロセスのみ**: 分散時の原子性(Redis + Lua)は扱っていない
- **単一キーのみ**: 実務ではユーザーごと・IPごとに Limiter を持つ(map + 掃除が必要)
- **二値判定のみ**: `golang.org/x/time/rate` にある `Wait`(空きが出るまで待つ)や `ReserveN` は未実装
- **sliding window log のメモリ管理は素朴**: スライスの詰め直しで済ませている

## 参考資料

- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate) — Go 公式の token bucket 実装。`Wait` / `Reserve` まで含む設計が学びになる
- [Cloudflare: How we built rate limiting capable of scaling to millions of domains](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/) — sliding window counter 方式の出典
- [Stripe: Scaling your API with rate limiters](https://stripe.com/blog/rate-limiters) — 実務での使い分け(rate limiter と load shedder)
