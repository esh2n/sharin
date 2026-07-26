# 教科書ページ計画

2026-07-25 決定。以降の章はすべてこれに従う。

## 決定事項

- 図は**自作 Vue コンポーネント**で統一する(mermaid は使わない)。`docs/components/figures/` に共通部品を置く
- 前提知識は**独立章**として作る(まず「二分探索木」「ディスクとページ」)
- 既存4章(rate-limiter / trace-sampling / llm-sampling / id-generation)の図補強は、新規3章の後に**一括監査**で行う

## 章テンプレート(全章共通)

1. **この章で作るもの** + 肝(3つ以内)
2. **前提**: 用語は初出で定義。既出章にあればリンク。前提が1節で収まらないなら前提章を作る
3. **本文**: 1概念1図を目安に図が主役、文章は図の補足。ASCII は補助まで
4. **コード**: スニペット埋め込み + トリッキーな箇所は「読みどころ」節(ビット演算は2進数の展開図)
5. **デモ**: 各方式・各概念の説明の直下(章末にまとめない)
6. **メリデメ + 実例**: 複数方式を比較する章では必須。実例は裏どり済みのみ
7. **簡略化したこと** / **参考資料**

## 図コンポーネント方針

- 共通ラッパ `FigureBox.vue`(枠・キャプション・ライト/ダーク両対応)
- 用途別の図部品は必要になった章で作り、使い回せるものは汎用化する(木、ページ配置、ビット配置など)
- 配色は VitePress テーマ変数のみ使う
- **箱を矢印で繋いだだけの図は禁止**。図は「何がどこで起きるか」の構造を持たせる
  (例: LaneSteps = ステップ×ファイルのレーン図)。番号はラベル埋め込みでなくチップで

## AI臭いデザインの禁止パターン(繰り返し指摘。仕組みで防ぐ)

- **角丸カード + 色付き左アクセントボーダーの併用は禁止**。角丸の角をアクセント線が
  なぞる構図は典型的な AI 生成っぽさ。左レールで状態を示すなら **角を落とす**
  (`border-radius: 0`)。「左ボーダー自体は良い、角丸なのがダメ」(2026-07-26 指摘)
- 意味のない装飾カード・テンプレ然としたグラデ/影・過剰な角丸を避ける。
  迷ったら既存の質素なコンポーネント(Summary の素の文章リード等)に倣う
- CSS で `border-radius` と `border-left`(色付き) が同じ要素に同居していたら赤信号。
  VRT 導入前でも `grep -rn "border-left" docs/components` で自己点検する

## デモ品質規約(2026-07-25 追加)

- 全デモは `DemoShell.vue` に載せる(タイトル + 状態バッジ + 整理されたコントロール行)
- コントロールは「入力(選択) → 実行(primary) → 補助(ghost)」の順で1行に。選択肢は
  select でなくセグメントコントロール
- 状態は裸のテキストで置かない。カード/バッジ/ログ行など、意味に合う器に入れる
- **ビルド後に必ず目視確認**してからコミットする。方式は下記「VRT の仕組み」に移行。
  それまでの暫定は scratchpad の使い捨て Playwright スクリプト(壊れやすい。playwright が
  グローバルから消える/node20対22 で動かなくなる事故あり)

## 全章に入れる必須要素(2026-07-25 夜 追加。既存章にも遡及適用)

1. **30秒要約**: 章の冒頭に `<Summary>` カード。「これは何か / なぜ大事か / 肝ひとこと」を
   3行で。忙しい人がここだけ読んで概要を掴める
2. **トップに全体図**: `index.md` にパーツの依存マップ(グループ別・状態バッジ・依存の矢印注記)。
   読む順と現在地が一目でわかる。ロードマップと同じ状態を反映する
3. **ダークモード目視も必須**: ライトに加えダークでも Playwright スクショ確認してからコミット
   (`colorScheme: 'dark'`)。テーマ変数だけ使っていれば基本崩れないが目視で担保する
4. **実行可能 Go リンク**: 各章に「その場で動かす」導線。各パーツに `example/` の
   self-contained な `main.go` を置き、`go run` の手順 + Go Playground 共有リンクを載せる
   (共有リンクは play.golang.org/share への POST で採番)

## VRT の仕組み(arekore packages/ui/vrt を sharin/VitePress 向けに移植) — **実装済み(2026-07-26)**

`docs/vrt/`(vrt.test.ts / commands.ts / setup.ts)+ `docs/vitest.config.mts` に実装済み。
`pnpm run vrt`(比較) / `pnpm run vrt:update`(基準採り直し)。使い方は `docs/vrt/README.md`。
基準画像は `docs/vrt/reference/<Demo>--<variant>--<theme>--<platform>.png`(74枚, darwin)。
`components/*Demo.vue` を glob 自動収集(登録漏れ防止)、variant 対応、必須 prop 未登録は
warn ガードで落ちる、light/dark、pixelmatch 差分0。RaftDemo は `vrt` prop で決定化。
**RaftDemo の角落とし修正は VRT で目視確認済み(角丸なし・状態色の左レール)**。
Docker 化(Linux 基準)は CI 導入時の follow-up(platform 軸は実装済みなので別基準を採るだけ)。

以下は移植時の設計メモ(参照用に残す)。

arekore の実績ある方式(vitest browser mode + pixelmatch, Docker で描画固定)を踏襲する。
React/Storybook 前提の部分だけ Vue/VitePress 向けに読み替える。

**参照元(arekore packages/ui/vrt/vrt.test.tsx + commands.ts)の4段構え:**
1. **拾い方**: `import.meta.glob("**/*.stories.tsx")` で全 stories を集め composeStories で実体化。
   部品を足せば自動で対象になり登録漏れが起きない
2. **描き方**: vitest browser mode(playwright chromium, viewport 720px 固定)で `#vrt-stage` に描く。
   台の幅は既定 320px、例外は table と diagram-group の 640px。アニメ・キャレットは CSS で止めて決定化
3. **撮り方**: `ctx.iframe.locator("#vrt-stage").screenshot()`。Modal と Toast は top-layer/fixed で
   台の外に出るので viewport 全体を撮る
4. **比べ方**: `<部品>/reference/<Story>--<theme>--<platform>.png` と pixelmatch。差分ピクセル 0 が条件。
   落ちたら `vrt/diffs/` に差分画像。**Docker で実行**(フォント描画を固定し pixel 完全一致を成立させる)

**sharin への移植方針(要検討・次セッションで着手):**
- 拾い方: sharin は stories が無い。`import.meta.glob("../components/*Demo.vue")` で全デモを自動収集
  (登録漏れ防止の思想は同じ)。props/variant が要るデモは小さな manifest を添える
- 描き方: vitest browser mode(`@vitest/browser` + playwright provider) + Vue で各デモを `#vrt-stage` に mount。
  VitePress のテーマ CSS 変数(`--vp-c-*`)を読み込ませないと色が出ないので、テーマ CSS を注入する。
  台幅は sharin のコンテンツ幅(≈688px)基準。アニメは CSS(`*{animation:none!important}`)で殺す
- 決定化: RaftDemo 等アニメするデモは `vrt` prop で自動 tick 停止+固定ステップ描画にする
  (RaftDemo は seed 固定 LCG なので tick 数さえ固定すれば完全再現)
- theme: light/dark を html の `.dark` クラス切替で2通り撮る。platform 軸は sharin では不要(web のみ)
- 比べ方: pixelmatch, 差分 0、`reference/<Demo>--<theme>.png`、落ちたら `diffs/`。Docker で描画固定
- スクリプト: `pnpm vrt` / `vrt:update`(reference更新) / ローカルで diffs を開いて閲覧

これを組んだら、まず RaftDemo の角落とし修正(角丸+左アクセント禁止対応)を再検証する。

## ロードマップ(対応ページ一覧)

状態: ✅公開済 / 🔜次に着手 / ⬜バックログ。順序は「前提 → 応用」の依存に従う。

### トラフィック制御と観測
| パーツ | 状態 | 備考 |
|---|---|---|
| rate-limiter | ✅ | Workers ライブデモ付き |
| trace-sampling | ✅ | head/tail シミュレータ |
| proxy | ⬜ | L4/L7、リバースプロキシ |

### データの持ち方 — **db 編を完成させるのが当面の主軸**
| パーツ | 状態 | 備考 |
|---|---|---|
| id-generation | ✅ | |
| binary-search-tree | ✅ | 前提 |
| lru-cache | ✅ | 前提 |
| disk-and-pages | ✅ | 前提 |
| btree | ✅ | |
| log-structured-kv | ✅ | |
| wal | ✅ | |
| buffer-pool | ✅ | |
| **btree-page-store** | 🔜 | B-Tree を buffer-pool の上に載せる(メモリ→ページ) |
| **btree + wal 統合** | 🔜 | クラッシュセーフなインデックス。db 編のクライマックス |
| **mini-sql** | 🔜 | 上記の上に parser + executor。SELECT/INSERT |
| hash-map | ⬜ | 衝突・リサイズ。等価検索の対抗馬 |
| bloom-filter | ⬜ | 確率的メンバシップ |
| skip-list | ⬜ | 確率的平衡。LSM の MemTable 用 |

### ネットワーク下層
| パーツ | 状態 | 備考 |
|---|---|---|
| tcp-ip | ⬜ | ソケットから。proxy 編の土台 |
| http-server | ⬜ | 生ソケットで HTTP を話す |
| dns | ⬜ | 名前解決 |

### 分散システム
| パーツ | 状態 | 備考 |
|---|---|---|
| raft | ✅ | フルRaft(選挙/複製/snapshot/メンバ変更)+2章。コミット済み |
| replication | ✅ | 単一リーダー ログシッピング(async/quorum/sync・ラグ・失敗時損失)+章。コミット済み |
| consistent-hashing | ✅ | リング + 仮想ノード(CRC32)。GetN で複製先。+章。コミット済み |
| distributed-lock | ⬜ | |

### メッセージングとRPC
| パーツ | 状態 | 備考 |
|---|---|---|
| message-queue | ✅ | ログ+オフセット・at-most/at-least-once・冪等=実質1回。+章。コミット済み |
| pubsub | ✅ | トピック fan-out・独立カーソル・FromBeginning/FromNow。+章。コミット済み |
| rpc | ✅ | protobuf風シリアライズ+フレーミング+ID相関+ctx timeout。+章。コミット済み |

### 暗号と認証
| パーツ | 状態 | 備考 |
|---|---|---|
| crypto | ⬜ | 共通鍵/公開鍵/ハッシュ/署名 |
| auth | ⬜ | crypto の応用 |
| blockchain | ⬜ | crypto の応用 |

### ランタイム内部
| パーツ | 状態 | 備考 |
|---|---|---|
| gc | ⬜ | mark-sweep GC |
| scheduler | ⬜ | goroutine 風の協調スケジューラ |
| event-loop | ⬜ | epoll 風の I/O 多重化 |
| wasm | ⬜ | バイトコード実行(lang 編と隣接) |

### LLMのなかみ
| パーツ | 状態 | 備考 |
|---|---|---|
| llm-sampling | ✅ | |
| llm | ⬜ | tensor 自作 → attention → GPT-2 forward。L サイズ |

### 画面が出るまで / フロント（TypeScript実装）
| パーツ | 状態 | 備考 |
|---|---|---|
| vdom | ✅ | 仮想DOM + diff/patch。h/mount/diff、パッチ関数方式。TS初パーツ。96.6%cov・型strict |
| mini-next | ✅ | SSR(renderToString)/ルーティング(:id)/ハイドレーション。vdomの上。100%cov |
| browser | ✅ | parse(HTML/CSS)→style(カスケード+継承)→layout(ブロック)→paint。robinson移植。99%cov |

### 計算機の土台
| パーツ | 状態 | 備考 |
|---|---|---|
| lang | ✅ | 字句→Pratt構文→ツリーウォーク評価。整数/真偽/let/if式/第一級関数/クロージャ。90.5%cov・-race |
| container | ✅ | namespace(PID/net/mount/UTS)+ cgroup(mem/pids、階層・OOM)の隔離モデル。99%cov・-race |
| os | ✅ | 協調スケジューラ(round-robin/yield/sleep/idle空転)。文脈=pc、非プリエンプティブ。96.9%cov・-race |

### ランタイム内部
| パーツ | 状態 | 備考 |
|---|---|---|
| gc | ✅ | mark-sweep GC。到達可能性=生死、tricolor(white/gray/black)、循環回収、black→white不変条件。100%cov・-race |
| event-loop | ✅ | epoll風 I/O多重化。ノンブロッキングFD(EAGAIN)+readiness一括問い合わせ(epoll_wait)+1スレッドのreactor。level-triggered/バックプレッシャ。100%cov・-race |
| bytecode | ✅ | バイトコードVM。lang のAST→平らな命令列+定数プール、スタックマシン(fetch-decode-execute)、if=条件ジャンプ+back-patching。langフロントエンド再利用。93.7%cov・-race |

## 直近の作業キュー(この順で)

1. **既存デモ7つを DemoShell に一括移行**: rate-limiter / trace / llm-sampling / idgen /
   btree / bst / kvlog。ライト+ダーク目視込み
2. **全11章に 30秒要約 + Go 実行リンクを遡及追加**
3. **db 編を進める**: btree-page-store → btree+wal 統合 → mini-sql
4. 以降の新パーツは常にこのテンプレート + 必須要素で書く
