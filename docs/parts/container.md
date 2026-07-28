<script setup>
import ContainerDemo from '../components/ContainerDemo.vue'
import Summary from '../components/Summary.vue'
import FigureBox from '../components/figures/FigureBox.vue'
</script>

# container(namespace と cgroup でつくる隔離)

<Summary>
「コンテナ」の正体を Go でモデル化する。Docker の中身は Linux カーネルの 2 つの機能の組み合わせだ。namespace は、PID やポートなど本来 1 つしかない資源を、コンテナごとに別々に見せる。だから 2 つのコンテナが同時に PID 1 を持ち、:80 も掴める。cgroup は、メモリやプロセス数の上限を予算として持ち、超えたら拒否する。コンテナは VM ではなく、カーネルを共有したまま制限を課したものだ。
</Summary>

## この章で作るもの

「コンテナは軽量な VM」という説明をよく聞くが、これは正確ではない。VM はカーネルもハードウェアも丸ごと仮想化するが、コンテナは**ホストのカーネルをそのまま共有する**。ではなぜ隔離されているように見えるのか。答えは Linux カーネルの2つの機能、**namespace** と **cgroup** だけにある。

<FigureBox caption="コンテナは VM ではない。1つのカーネルを共有したまま、プロセスに『制限された見え方(namespace)』と『資源の予算(cgroup)』を与えたもの。web と db はどちらも自分の init を PID 1 だと思っており、どちらも :80 を掴めるが、その下では host が1つの PID 空間・1つのメモリプールを握っている">

```
        ┌──────────────── Host(1つのカーネル)────────────────┐
        │  global PID を通し番号で発番 / ルート cgroup で総量管理  │
        │                                                      │
        │   ┌── Container: web ──┐      ┌── Container: db ──┐   │
        │   │ PID  : init = 1    │      │ PID  : init = 1   │   │
        │   │ net  : :80         │      │ net  : :80        │   │
        │   │ mount: /data = volA│      │ mount: /data = volB│  │
        │   │ cgroup: mem 256MiB │      │ cgroup: mem 256MiB│   │
        │   └────────────────────┘      └───────────────────┘   │
        └──────────────────────────────────────────────────────┘
```

</FigureBox>

肝は2つ:

1. **namespace = グローバル資源の独立した見え方**: PID・ポート・マウント・ホスト名を、コンテナごとに別インスタンスに見せる
2. **cgroup = 階層的な資源の予算**: メモリとプロセス数の上限を木で持ち、超えたら全か無かで拒否する

## ① namespace: 見え方を翻訳する

隔離の第一歩は「見え方」を分けること。ここで大事なのは、namespace は情報を消すのではなく、別の見え方に翻訳するという点だ。一番わかりやすいのが PID 名前空間になる。

ホストのカーネルは全プロセスに通し番号(global PID)を振る。だが PID 名前空間の中では、プロセスは 1 から数え直した local PID で見える。新しい名前空間の最初のプロセスは必ず PID 1 で、これがコンテナの `init` だ。global と local を相互に翻訳する写像を持つだけでいい:

<<< ../../foundations/container/pidns.go#pidns{go}

同じ仕組みが他の資源にも効く。ネットワーク名前空間は独立したポート表を持つので、別々のコンテナが同時に `:80` を bind できる(同じ名前空間内での二重 bind だけが衝突する):

<<< ../../foundations/container/netns.go#netns{go}

マウント名前空間は独立したマウント表を持ち、パスを最長一致で実体に解決する。だから2つのコンテナが同じ `/data` に別々のボリュームを見られる:

<<< ../../foundations/container/mountns.go#mountns{go}

ホスト名(UTS 名前空間)も同じ発想の最小版で、コンテナごとに1つの文字列を持つだけ。本来グローバルな1つの資源を、コンテナごとに別インスタンスに見せる。これが namespace の一貫した正体だ。

## ② cgroup: 資源に予算をつける

見え方を分けても、資源の奪い合いは防げない。1つのコンテナがメモリを食い尽くせば、同じカーネルを共有する全員が巻き添えになる。そこで **cgroup**(control group)が資源に上限をつける。

肝は階層であること。cgroup は木構造で、親の制限は子孫すべてに効く。メモリを計上(charge)するたびに、自分から先祖までさかのぼって「どこかの上限を超えないか」を確認し、超えるなら何も変えずに拒否する(全か無か)。使用量は親にロールアップされるので、親は配下の合計を常に把握できる:

<<< ../../foundations/container/cgroup.go#cgroup{go}

この「先祖まで確認してから、先祖まで加算する」の2周ループが要。1周目で全階層の上限をチェックし、通ったら2周目で全階層に反映する。途中で失敗しても何も汚さない。プロセス数(`pids.max`)もまったく同じロジックで上限をかけられる。

## ③ 組み立て: Host と Container

`Host` は1つの共有カーネル。global PID を全コンテナ通しで発番し、ルート cgroup でマシン全体のメモリを束ねる。`Container` は namespace 一式と、host 配下の子 cgroup を束ねたもの:

<<< ../../foundations/container/container.go#container{go}

`Spawn` の atomic 性に注目してほしい。プロセス数とメモリを順に計上し、途中でどちらかの上限に当たったら、それまでの計上を戻して起動そのものを失敗させる。半端に資源を掴んだまま失敗することがない。これが「OOM でコンテナ内のプロセス起動が弾かれる」の正体だ。

### 動かす

下のデモは、この隔離モデルを**そのままブラウザで**動かしている(Go 実装の考え方を JS に移植)。2つのコンテナ web / db にプロセスを起こし、メモリ上限まで積んでいける。両方の `init` が PID 1 なこと、両方が :80 を掴めること、そして片方がメモリ上限に当たっても(OOM バッジ)もう片方は無事なことを確かめてほしい。下段には host から見た global PID の通し番号が並ぶ。これが**1つのカーネルを共有している**証拠だ。

<ContainerDemo />

## 設計の観点: コンテナ vs VM、そして隔離の強さ

- **コンテナ ≠ VM**: VM はハイパーバイザの上でカーネルごと動かす(強い隔離・重い)。コンテナはホストのカーネルを共有し、namespace + cgroup で隔離する(軽い・起動が速い・が隔離は相対的に弱い)。「軽量 VM」という比喩は起動の軽さを指すが、仕組みは根本的に違う
- **隔離の弱点**: カーネルを共有する以上、カーネルの脆弱性を突かれるとコンテナの壁を越えられる(コンテナエスケープ)。だから信頼境界をまたぐマルチテナントでは、gVisor(ユーザ空間カーネル)や Firecracker / Kata(軽量 VM)で「カーネルも分ける」層を足すことがある
- **なぜ起動が速いか**: OS を起動しない。namespace を作ってプロセスを1つ `exec` するだけ。ミリ秒〜秒で立ち上がる。これがオートスケールや FaaS(サーバレス)を支える
- **イメージのレイヤ**: 本章では触れないが、コンテナイメージは overlay ファイルシステムの積層(読み取り専用レイヤ + 書き込み可能な最上層)。マウント名前空間の応用で、起動を軽く・ディスクを共有する

この章の要点は「コンテナは namespace(見え方)+ cgroup(資源)+ ホストカーネル共有。VM と違いカーネルを分けないので軽いが隔離は弱く、強い隔離が要るなら軽量 VM を足す」に尽きる。

## メリット・デメリットと実例

| 隔離方式 | 仕組み | 隔離の強さ | 起動 | 実例 |
|---|---|---|---|---|
| プロセスのみ | 同じ OS で普通に起動 | 弱い | 最速 | 従来のデーモン |
| コンテナ | namespace + cgroup、カーネル共有 | 中 | 速い(ms〜s) | Docker、containerd、Kubernetes Pod |
| 軽量 VM | 最小化した VM を1つずつ | 強い | 中(〜100ms) | Firecracker(AWS Lambda)、Kata |
| フル VM | ハイパーバイザ + 独立カーネル | 最強 | 遅い(秒〜) | EC2、VMware |

裏どり:

- **Docker / containerd**: `clone(2)` に `CLONE_NEWPID` などの flag を渡して namespace を作り、`/sys/fs/cgroup` に書いて資源制限をかける。本章の `Spawn` はその atomic な計上をモデル化したもの
- **Kubernetes の Pod**: 複数コンテナで **PID/ネットワーク名前空間を共有**する単位。同じ Pod 内なら `localhost` で通信でき、`:80` を1つのコンテナしか使えない。本章の「同一名前空間の二重 bind は衝突」がそのまま効く
- **AWS Lambda / Fargate**: 起動の速さと隔離の強さを両立するため、コンテナではなく **Firecracker 軽量 VM** を使う。マルチテナントで「カーネルも分ける」判断の実例
- **cgroup v2 の pids.max / memory.max**: 本章の `PidsLimit` / `MemLimit` と同じ。fork 爆弾を pids.max で止め、OOM を memory.max で制御する

## 簡略化したこと

- **syscall を使わない**: 本物は `clone`/`unshare` と `/sys/fs/cgroup` で実現する。ここはその仕組みのモデルで、実プロセスは作らない
- **namespace は4種類だけ**: PID・ネットワーク・マウント・UTS(ホスト名)。user / IPC / time / cgroup 名前空間は省略
- **cgroup は memory と pids のみ**: CPU シェア・ブロック I/O・帯域制限は無し
- **メモリは宣言値**: プロセスが使う量を引数で受け取る。実測・ページング・スワップは無し
- **ネットワークは bind のみ**: veth・ブリッジ・ルーティング・NAT は扱わない
- **イメージ・レイヤ FS なし**: overlayfs によるイメージの積層は別の話題(マウント名前空間の応用)

## 参考資料

- Linux man pages: [`namespaces(7)`](https://man7.org/linux/man-pages/man7/namespaces.7.html) / [`cgroups(7)`](https://man7.org/linux/man-pages/man7/cgroups.7.html) — 一次資料
- Liz Rice, *Container Security*(O'Reilly)/ "Building a container from scratch in Go"(GOTO 講演) — Go でゼロから作る名講演
- [opencontainers/runc](https://github.com/opencontainers/runc) — namespace と cgroup を実際に叩く低レベルランタイム
- 実装: [foundations/container](https://github.com/esh2n/sharin/tree/main/foundations/container)
