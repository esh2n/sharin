# sharin

車輪の再発明で学び直すリポジトリ。

AIで作れるものの幅は広がったが、そのぶん中身がブラックボックスになりやすい。
rate limiter を1から書く、サンプリングを logits から計算する、といった機会を
意図的に作り、パーツごとに「簡略化しつつ肝は外さない」最小実装を積む。

## 進め方の型

1パーツ = 1ディレクトリ。各パーツは次の4点セットを持つ。

```
<part>/
├── README.md   # 肝(3行以内) / 簡略化したこと / 本物との差分 / 参考資料
├── 実装         # 最小限。ライブラリに逃げない部分を明確にする
├── テスト       # 挙動を言葉で説明できることの証明
└── notes.md    # 作りながらの気づき(任意)
```

- 「簡略化したこと」を明記するのが最重要。何を捨てたか自覚していれば簡略化は嘘にならない。
- 言語は原則 Go。ブラウザ/VDOM 系のみ TypeScript。
  LLM も Go で書く: numpy に逃げない = 行列積や softmax も自作する、それ自体が車輪の再発明。
  Python は HF transformers の出力と突き合わせる検算スクリプト用にのみ許可。

## 教科書サイト

実装と並行して docs/ に教科書を書き、Cloudflare Pages で公開する。
公開URL: https://sharin-2a1.pages.dev

- スタックは VitePress を想定。`<<< @/../rate-limiter/token_bucket.go#region` 形式で
  実コードをスニペットとして埋め込めるため、教科書のコードと動くコードが乖離しない。
- 章構成はバックログのパーツと1対1。本文(なぜそう動くか、図、落とし穴)は docs/ 側に書き、
  各パーツの README.md は「肝 / 簡略化したこと / 章へのリンク」の要約に留める。
- デプロイは GitHub Actions から wrangler で Cloudflare Pages へ。main は本番、PR はプレビューURL。
- Cloudflare を選んだ理由: PR ごとのプレビューと、章に添える「実際に叩けるライブデモ」を
  Workers で同居させられるため。

## バックログ

サイズ目安: S = 1〜2日 / M = 数日〜1週間 / L = 週単位以上

### Tier 1: 小さく完結する(最初の弾み車)

| パーツ | 作るもの | 肝 | サイズ |
|---|---|---|---|
| rate-limiter | token bucket / leaky bucket / fixed window / sliding window log の比較実装 | バーストの許し方と平滑化のトレードオフ。時計と原子性 | S |
| id-generation | UUIDv4 / UUIDv7 / ULID / Snowflake を自作、衝突確率も計算 | 時刻 + 乱数 + ノードIDの配合。ソート可能性がなぜ効くか | S |
| llm-sampling | logits 配列から greedy / temperature / top-k / top-p / min-p を実装 | softmax の温度が分布をどう歪めるか。生成は毎トークン確率実験 | S |
| trace-sampling | 簡易トレーサー + head-based / tail-based サンプラー、エラートレース捕捉率の比較 | 全量保存できない中で何を残すか。tail は「完結を待つバッファ」の代償を払って価値の高いトレースを確実に残す | S〜M |
| data-structures | hash map(衝突・リサイズ) / B-Tree / LRU / bloom filter / skip list | 計算量の理屈を「自分で書いたから」で説明できる状態にする。DB編の下ごしらえ | S×複数 |

### Tier 2: サーバー側の仕組み

| パーツ | 作るもの | 肝 | サイズ |
|---|---|---|---|
| proxy | TCP フォワードプロキシ、HTTP リバースプロキシ(ヘッダ書換・LB付き)、CONNECT トンネル | L4 と L7 の違い。ストリーム中継と X-Forwarded-For の意味 | M |
| crypto | XOR暗号、AES(モードの違い)、Diffie-Hellman、RSA(小さな素数で)、SHA-256、最後にトイTLSハンドシェイク | 共通鍵と公開鍵の役割分担。「暗号化」と「署名」と「ハッシュ」は別物 | M |
| auth | セッション+Cookie 自作、JWT(HMAC署名を手計算)、OAuth2 の3役(client / auth server / resource server)をトイ実装 | 認証と認可の違い。署名検証がなぜサーバー間の信頼になるか。crypto 編の応用 | M |
| db | append-only KV から始めて WAL、B-Tree ページストア、ミニSQL(parser + executor)、インデックス、最後にMVCCの触り | 永続化とクラッシュ耐性。インデックスが速い理由を B-Tree 自作済みの目で見る | M〜L(段階制) |
| blockchain | ハッシュチェーン + PoW、merkle tree、UTXO トランザクション(署名は crypto 編の応用)、簡易 P2P 同期 | ブロックチェーンは「改竄コストを計算量で買う追記専用ログ」。難易度調整の意味 | M |
| container | chroot / pivot_root、namespaces(UTS/PID/mount)、cgroups v2、overlayfs、イメージ tar 展開までのミニ runc | コンテナは VM ではなくただのプロセス。カーネル機能の組み合わせにすぎないこと。Linux 必須(macOS では Lima 等の VM 内で) | M |

### Tier 3: フロントエンドの中身

| パーツ | 作るもの | 肝 | サイズ |
|---|---|---|---|
| vdom | mini React: createElement / render / diff / 再レンダリング / useState をクロージャで | VDOM diff は木の比較アルゴリズム。hooks が「呼び出し順の配列」で動くこと | M |
| mini-next | vdom の上に SSR と hydration、file-based routing の最小版 | サーバーで作った HTML にクライアントでイベントを「接ぎ木」するのが hydration | M |
| browser | HTML parser、DOM ツリー、CSS カスケード、レイアウト(ブロック/インライン)、矩形描画まで | パースからピクセルまでのパイプライン。robinson 系のトイエンジン | L |

### Tier 4: 大物

| パーツ | 作るもの | 肝 | サイズ |
|---|---|---|---|
| llm | Go で自作 tensor(float32 slice + shape)、matmul / layernorm / GELU、BPE トークナイザ、GPT-2 相当の forward pass、KV cache。Python は検算のみ | attention は行列積の塊。生成が逐次で遅い理由と KV cache が効く理由。llm-sampling 編と接続 | L |
| lang | lexer、Pratt parser、AST インタプリタ、(余力で bytecode VM) | 再帰下降パースと評価器。Monkey 言語(Writing an Interpreter in Go)系 | L |
| os | RISC-V のトイOS: ブート、割り込み、ページング、コンテキストスイッチ、簡易syscall | 特権レベルとハードウェアとの契約。「OSは割り込みで駆動されるイベントループ」 | L |

## 着手順の目安

Tier 1 から。最初の1本は rate-limiter(このリポジトリを作った動機そのもの)。
Tier 内は自由順だが、依存関係として以下だけ意識する。

- data-structures の B-Tree は db の前に
- crypto は auth と blockchain の前に
- llm-sampling は llm の前に(どちらが先でも成立はする)
- trace-sampling は rate-limiter の後だと「何を通すか/何を残すか」の対比が効く
- vdom は mini-next の前に

このリストは固定ではない。やりたいものが増えたらバックログに足す。
