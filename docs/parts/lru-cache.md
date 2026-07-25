<script setup>
import LruDemo from '../components/LruDemo.vue'
import FigureBox from '../components/figures/FigureBox.vue'
import Summary from '../components/Summary.vue'
</script>

# LRUキャッシュ

> 実装: [`data-structures/lru/`](https://github.com/esh2n/sharin/tree/main/data-structures/lru) / 実行: `go test ./data-structures/lru/`

<Summary>
決まった数しか入らないキャッシュで、いっぱいになったら「一番長く使っていないもの」から捨てる作戦を作る。map で場所を一発で引き、双方向リストで使った順番を保つ、という二段構えがLRUの正体。次のバッファプールが、これをそのまま「どのページを捨てるか」に使う。
</Summary>

## この章で作るもの

容量が決まったキャッシュで「溢れたら一番長く使っていないものを捨てる」戦略、
LRU(Least Recently Used)を実装する。次章の[バッファプール](./buffer-pool)が
これをそのまま追い出し器として使う。

この章の肝は3つ。

- LRU は **map + 双方向リスト**の組み合わせ。単独ではどちらも足りない
- map で「そのキーがどこにあるか」を O(1) で引く
- リストで「最近使った順」を O(1) で並べ替える

## なぜ map だけでは足りないか

「キー → 値」なら map で十分。しかし LRU には
「**一番長く使っていないものはどれか**」という問いに即答する責任がある。
map はキーの順序を持たないので、これに答えるには全走査するしかない — O(n)。

そこで「最近使った順」に並んだ列を別に持つ。これを配列でやると、
真ん中の要素を先頭に動かすのに後続を全部ずらすので O(n)。
**双方向リスト**なら、前後のポインタを繋ぎ替えるだけで O(1) で動かせる。

<FigureBox caption="map はキーからリストのノードへの近道。リストは最近使った順を保つ。この二段構えで get も put も O(1)">

```
      ┌─────────── map ───────────┐
      │  "a"→●   "b"→●   "c"→●     │   キーからノードへ O(1) で飛ぶ
      └────┼───────┼───────┼───────┘
           ▼       ▼       ▼
  先頭 ⇄ [ c ] ⇄ [ b ] ⇄ [ a ] ⇄ 末尾      双方向リスト(最近使った順)
  最近使った側              次に捨てる側
```

</FigureBox>

## 実装

ノードは双方向リストの要素。値だけでなく前後のポインタとキーを持つ
(追い出すとき map からも消すためにキーが要る)。

<<< ../../data-structures/lru/lru.go#entry{go}

Get は「値を返す」だけの操作に見えて、実は**触ったノードを先頭に動かす**のが本体。
これを忘れると「最近使った」が更新されず、ただの map になってしまう。

<<< ../../data-structures/lru/lru.go#get{go}

Put は容量を超えたら末尾(一番使われていない)を追い出す。
追い出した要素を**捨てずに呼び出し側へ返す**のが設計の勘所で、
次章のバッファプールが「捨てる前にディスクへ書き戻す」後始末をするために使う。

<<< ../../data-structures/lru/lru.go#put{go}

**試してみる**: A→B→C と入れると満杯。そこで A を使う(Get)と A が先頭に戻り、
次に D を入れたとき追い出されるのは A ではなく B になる。
「使うと生き延びる」のが LRU。

<LruDemo />

## メリット / デメリット

**メリット**

- get / put が両方 O(1)。実装も短い
- 「最近使ったものは近いうちにまた使う」(時間的局所性)が成り立つ場面で高いヒット率

**デメリット**

- **スキャン耐性がない**: 大きなデータを1回だけ舐める処理(全件バッチ等)が走ると、
  二度と使わないページで有用なページを全部押し出してしまう。
  実物の DB は midpoint 挿入(新入りをいきなり先頭に置かない)等で対策する
- ポインタとキーのぶんメモリを食う(値そのものより重いこともある)

**実例**

- Go の [`groupcache/lru`](https://github.com/golang/groupcache)、
  [`hashicorp/golang-lru`](https://github.com/hashicorp/golang-lru) — この章と同じ map+list 構成
- Redis の `maxmemory-policy allkeys-lru`(厳密には近似 LRU。全ノードを繋ぐ代わりに
  サンプリングで近い効果を出す)
- 次章の**バッファプール** — OS のページキャッシュや DB のバッファ管理の土台

## 簡略化したこと

- **スレッド安全ではない**: 利用側でロックする前提(バッファプールがそうしている)
- **TTL・重み付きサイズなし**: エントリ数だけで容量を測る
- **スキャン耐性なし**: 上記の通り、素の LRU のまま

## 参考資料

- [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru) — 実運用される Go 実装。
  ARC / 2Q など LRU の改良版も入っている
- [Redis: Key eviction](https://redis.io/docs/latest/develop/reference/eviction/) — 近似 LRU の実際
