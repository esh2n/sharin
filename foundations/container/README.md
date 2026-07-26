# container — namespace と cgroup でつくる隔離

「コンテナ」の正体を Go でモデル化する。計算機の土台編の2つ目のパーツ。
Docker や runc が Linux カーネルの機能で実現している隔離——**namespace**(見え方の隔離)と
**cgroup**(資源の予算)——を、syscall を使わず純粋なデータ構造で決定的に再現する。

教科書の章: [container](https://sharin-2a1.pages.dev/parts/container)

## これは何か

コンテナは VM ではない。**1つのカーネルを共有したまま**、プロセスに

- 制限された「見え方」= **namespace**(PID / ネットワーク / マウント / ホスト名)
- 資源の「予算」= **cgroup**(メモリ・プロセス数)

を与えたものにすぎない。この2つを組み合わせると、同じマシンの上で

- 各コンテナの `init` がどちらも **PID 1** に見える(でも host の global PID は別)
- 2つのコンテナが同時に **:80 を bind** できる
- 同じ `/data` に**別々のボリューム**が見える
- 片方のコンテナが**メモリを使い切っても**(OOM)、もう片方は無事

が成り立つ。これを `Host` / `Container` / `CGroup` の3つで表す。

```
        ┌──────────────── Host(1つのカーネル)────────────────┐
        │  global PID を通し番号で発番 / ルート cgroup で総量管理  │
        │                                                      │
        │   ┌── Container: web ──┐      ┌── Container: db ──┐   │
        │   │ PID ns: init=1     │      │ PID ns: init=1    │   │
        │   │ net ns: :80        │      │ net ns: :80       │   │
        │   │ mnt ns: /data=volA │      │ mnt ns: /data=volB│   │
        │   │ cgroup: mem 256MiB │      │ cgroup: mem 256MiB│   │
        │   └────────────────────┘      └───────────────────┘   │
        └──────────────────────────────────────────────────────┘
```

## 肝は2つ

1. **namespace = グローバル資源の独立した見え方**: PID・ポート・マウント・ホスト名という
   「本来1つしかない資源」を、コンテナごとに別インスタンスに見せる。PID 名前空間なら
   global PID(host が振る通し番号)を local PID(コンテナ内は 1 から)に翻訳する
2. **cgroup = 階層的な資源の予算**: メモリとプロセス数の上限を木構造で持つ。charge の
   たびに自分から先祖までさかのぼって上限を確認し、超えるなら「全か無か」で拒否する(OOM)。
   使用量は親にロールアップされるので、親は配下の合計を常に把握できる

## ファイル

- `pidns.go` — PID 名前空間。global PID ⇄ local PID の翻訳。最初のプロセスが local 1
- `netns.go` — ネットワーク名前空間。ポートの bind。別名前空間なら同じポートが使える
- `mountns.go` — マウント名前空間。マウント表を最長一致で解決。既定は rootfs
- `cgroup.go` — 階層 cgroup。メモリ・プロセス数の上限と計上、先祖へのロールアップ
- `container.go` — `Host`(共有カーネル)/ `Container`(namespace 一式 + cgroup)の組み立て

## 設計メモ

- **Host は1つのカーネル**: global PID を全コンテナ通しで発番し、ルート cgroup で
  マシン全体のメモリを束ねる。コンテナに制限を付けなくても、host 全体の量に縛られる
- **全か無か(atomic)**: `Spawn` は pids とメモリの計上を試み、途中で上限に当たったら
  それまでの計上を戻して起動そのものを失敗させる。半端に資源を掴んだまま失敗しない
- **翻訳としての namespace**: 隔離は「情報を消す」のではなく「別の見え方に翻訳する」。
  PID 名前空間は global→local の写像、マウント名前空間はパス→実体の写像

## 簡略化したこと

- **syscall を使わない**: 本物は `clone(CLONE_NEWPID|CLONE_NEWNS|…)` や
  `/sys/fs/cgroup` への書き込みで実現する。ここはその仕組みのモデルで、実プロセスは作らない
- **user / IPC / time 名前空間は省略**: PID・ネットワーク・マウント・UTS(ホスト名)だけ
- **cgroup は memory と pids のみ**: CPU シェア・ブロック I/O・ネットワーク帯域は無し
- **メモリは「宣言値」**: プロセスが使うメモリを引数で受け取る。実測やページングは無し
- **ネットワークは bind のみ**: 実際の仮想イーサネット(veth)・ルーティングは扱わない

## 動かす

```bash
go test ./foundations/container/ -race -cover
go vet ./foundations/container/
```

## 参考

- Linux man pages: `namespaces(7)`, `pid_namespaces(7)`, `cgroups(7)`
- Liz Rice, *Container Security* / "Building a container from scratch in Go"(講演)
- opencontainers/runc — namespace と cgroup を実際に叩く低レベルランタイム
