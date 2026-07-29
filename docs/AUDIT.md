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
| id-generation | ⬜ | |
| binary-search-tree | ⬜ | |
| lru-cache | ⬜ | |
| disk-and-pages | ⬜ | |
| btree | ⬜ | |
| log-structured-kv | ⬜ | |
| wal | ⬜ | |
| buffer-pool | ⬜ | |
| btree-page-store | ⬜ | |
| btree-wal | ⬜ | |
| secondary-index | ⬜ | |
| mini-sql | ⬜ | |
| mvcc | ⬜ | |
| hash-map | ⬜ | |
| bloom-filter | ⬜ | |
| skip-list | ⬜ | |
| inverted-index | ⬜ | |
| distributed-intro | ⬜ | |
| logical-clock | ⬜ | |
| causal-broadcast | ⬜ | |
| total-order | ⬜ | |
| raft | ⬜ | |
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
