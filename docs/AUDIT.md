# 全章監査台帳(読みやすさ)

2026-07-29 ユーザ指摘(原文):

> ラウンドロビンとかハッシュとかの説明なく話が進んでいて非常にわかりにくいページだった。そうなると他もクオリティが気になってしまう

> これも非常にわかりにくい。文章がAIくさくわかりにくいのとIPがいきなりはいってきてる。もちろん全部みてね

> ぜんぶのぺーじをみてね?/ 絶対に全部見切ってね?

> デモとか作るものの前にその説明がないとわからんでしょ?

## 検査基準

1. **未定義語の先出し**: Summary・冒頭・図で、その章がまだ説明していない用語を使っていないか。
   前提章のリンクだけで済ませず、1行でも言葉の説明を置く(例: TCP 章の「IP」)。
2. **AI臭い文型**: 「鍵は A、B、C、D だ」「〜を作るのが X だ」の定型連打、体言止めの連続、
   自問自答、結論を言わない列挙。1文1義に書き直す。
3. **説明の前に成果物**: デモ・図・「この章で作るもの」が、必要な概念の説明より先に来ていないか。
   説明 → 実装 → デモの順に直す。
4. 既存の散文ルール(Summary 150-200字 / strong 0 / —— 禁止 / 禁止語)は維持。

方法: 1章ずつ全文を読む。基準に触れたら書き直す。済んだら下の表を ✅ にして
1行で何を直したかを書く。「指摘された章だけ直す」は禁止(feedback_no_pointwise_fixes)。

一括処置済み(2026-07-29):
- 行頭定型「肝は3つ:」「この章の肝は3つ。」(119章)を全章から機械的に除去
- 章末の丸ごと要約段落「この章の要点は「…」に尽きる。」「〜と言えるかが要。」(25章)を削除
地の文の「〜が肝で」「〜に尽きる」は内容を持つ通常の日本語なので個別判断。
この一括置換は ✅ の代わりにはならない(各章の全文読解は別途必要)。

## 進捗

| 章 | 状態 | 直したこと |
|---|---|---|
| load-balancer | ✅ | 4方式を①〜④で説明してから比較。実装も方式ごとに分割。RRの崩れ方を実測 |
| tcp | ✅ | パケット→IP→TCPの順で土台から説明。番号→返事→送り直し→窓を説明後にコード。「鍵は〜だ」「〜に尽きる」定型と◎△✕表を排除 |
| rate-limiter | ✅ | 合格。方式ごとに物理モデル→コード→メリデメ→実例→デモの順が守られている。手本のまま |
| circuit-breaker | ✅ | half-open を図の前に定義。continuously の英語混入・degrade→縮退・「肝は3つ」「鍵になる」定型を排除 |
| retry-backoff | ✅ | 「3種類ある」を実装どおり(なし/full/equal)に明示。「肝は3つ」定型を排除 |
| metrics | ✅ | Summary から p99・バケットの先出しを排除(現象で言い直し)。バケットは初出で定義 |
| timeseries | ✅ | Summary と本文の Pod を「サービスの複製」で定義。p99 を初出で言い直し。「肝は3つ」定型を排除 |
| tracing | ✅ | Summary から span・クリティカルパスの先出しを排除。「束ねる鍵」定型を排除 |
| trace-sampling | ✅ | trace/span の前提節を「作るもの」より前に移動(要点リストが span を先出ししていた) |
| http-server | ✅ | Summary の TCP を役割つきで言い直し。「自作 TCP は発展」の古い記述を tcp/http2/tls-handshake 章へのリンクに修正 |
| http2 | ✅ | ほぼ合格。「肝は3つ」定型のみ排除 |
| websocket | ✅ | Summary の未定義語(Accept計算/フレーミング/マスク)を現象で言い直し。SHA-1/base64 に1行の説明 |
| congestion | ✅ | Summary と図の cwnd/ssthresh に言い直しを追加。「肝は3つ」「公平の鍵」定型を排除 |
| dns | ✅ | UDP の1行説明が章内に無かったのを冒頭で定義。Summary も接続なし1発の言い回しに |
| proxy | ✅ | SSL終端を TLS 終端として初出で定義。「3章で完結」の古い記述を現構成に合わせ tcp を追加 |
| id-generation | ✅ | UUIDv4 節が説明なしでコードから始まっていたのを修正。誕生日のパラドックスに1行の説明 |
| binary-search-tree | ✅ | 合格。用語の前提節・説明→実装→デモの順とも手本どおり。定型のみ除去 |
| lru-cache | ✅ | 合格。定型のみ除去 |
| disk-and-pages | ✅ | 合格。B-Tree デモ言及の既読前提(「見た」)を時制修正 |
| btree | ✅ | 合格。前提章の明示・順序とも手本どおり |
| log-structured-kv | ✅ | 合格 |
| wal | ✅ | 合格。冪等・原子性とも説明後に使用 |
| buffer-pool | ✅ | 合格。「前章の LRU」を実際の章順に合わせリンクに修正 |
| btree-page-store | ✅ | 合格 |
| btree-wal | ✅ | 合格。「鍵は txn」の定型のみ言い換え |
| secondary-index | ✅ | ミニSQL既読前提だったためサイドバー順を mini-sql→secondary-index に入替。①冒頭に導入文を追加 |
| mini-sql | ✅ | 合格。Summary の語の重複のみ修正 |
| mvcc | ✅ | Summary の lost update を現象で言い直し。「に尽きる」要約段落を削除 |
| hash-map | ✅ | 合格 |
| bloom-filter | ✅ | 冒頭リスト後に紛れた古い箇条書きの残骸3行を削除。Summary の偽陽性率に言い直し |
| skip-list | ✅ | 合格 |
| inverted-index | ✅ | 「に尽きる」要約段落を削除。他は合格 |
| distributed-intro | ✅ | 合格。部分故障→split brain→過半数の順で説明が積み上がっている |
| logical-clock | ✅ | Lamport・ベクタークロックの名前を初出で導入。後続章(leader-election/replication)の既読前提を修正 |
| causal-broadcast | ✅ | 後続章(gossip/Raft)を過去形で参照していた3箇所を修正 |
| total-order | ✅ | 後続のクォーラム章への既読前提を修正 |
| raft | ✅ | 「前章」の誤参照3箇所を章名に修正。CAP を初出で定義(未説明のまま「前章のCAP」と参照していた) |
| replication | ⬜ | |
| crdt | ⬜ | |
| consistent-hashing | ⬜ | |
| quorum | ⬜ | |
| anti-entropy | ⬜ | |
| gossip | ⬜ | |
| distributed-lock | ⬜ | |
| distributed-txn | ⬜ | |
| capacity-estimation | ⬜ | |
| message-queue | ⬜ | |
| pubsub | ⬜ | |
| rpc | ⬜ | |
| event-sourcing | ⬜ | |
| cqrs | ⬜ | |
| vdom | ⬜ | |
| reactivity | ⬜ | |
| store | ⬜ | |
| bundler | ⬜ | |
| mini-next | ⬜ | |
| browser | ⬜ | |
| lang | ⬜ | |
| type-inference | ⬜ | |
| numbers | ⬜ | |
| allocator | ⬜ | |
| virtual-memory | ⬜ | |
| regex | ⬜ | |
| container | ⬜ | |
| cpu-throttling | ⬜ | |
| oom | ⬜ | |
| os | ⬜ | |
| gc | ⬜ | |
| event-loop | ⬜ | |
| bytecode | ⬜ | |
| scheduler | ⬜ | |
| wasm | ⬜ | |
| apiserver | ⬜ | |
| kubelet | ⬜ | |
| etcd-ops | ⬜ | |
| reconcile | ⬜ | |
| pod-scheduler | ⬜ | |
| autoscaler | ⬜ | |
| custom-metrics | ⬜ | |
| pod-lifecycle | ⬜ | |
| init-container | ⬜ | |
| probe | ⬜ | |
| rollout | ⬜ | |
| cluster-autoscaler | ⬜ | |
| topology | ⬜ | |
| service | ⬜ | |
| pdb | ⬜ | |
| preemption | ⬜ | |
| statefulset | ⬜ | |
| csi | ⬜ | |
| job | ⬜ | |
| daemonset | ⬜ | |
| config | ⬜ | |
| ingress | ⬜ | |
| gateway-api | ⬜ | |
| network-policy | ⬜ | |
| rbac | ⬜ | |
| admission | ⬜ | |
| pod-security | ⬜ | |
| quota | ⬜ | |
| leader-election | ⬜ | |
| operator | ⬜ | |
| hashing | ⬜ | |
| symmetric-cipher | ⬜ | |
| key-exchange | ⬜ | |
| crypto | ⬜ | |
| tls-handshake | ⬜ | |
| auth | ⬜ | |
| oauth | ⬜ | |
| blockchain | ⬜ | |
| utxo | ⬜ | |
| evm | ⬜ | |
| rollup | ⬜ | |
| lightning | ⬜ | |
| substrate | ⬜ | |
| bpe | ⬜ | |
| tensor | ⬜ | |
| attention | ⬜ | |
| transformer | ⬜ | |
| rope | ⬜ | |
| rmsnorm-swiglu | ⬜ | |
| attention-variants | ⬜ | |
| moe | ⬜ | |
| ssm | ⬜ | |
| patchify | ⬜ | |
| mini-gpt | ⬜ | |
| llm-architecture | ⬜ | |
| gpt-lineage | ⬜ | |
| claude-lineage | ⬜ | |
| gemini-lineage | ⬜ | |
| open-models | ⬜ | |
| reasoning-models | ⬜ | |
| llm-training | ⬜ | |
| llm-sampling | ⬜ | |
| inference | ⬜ | |
| quantization | ⬜ | |
| gptq | ⬜ | |
| pruning | ⬜ | |
| embedding-search | ⬜ | |
| agent-harness | ⬜ | |

未着手 = ⬜、着手中 = 🔧、済 = ✅。この表が正。セッションをまたいでもここから再開する。
