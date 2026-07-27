# allocator — メモリアロケータ(malloc/free)

「n バイトください」の裏側。free list 方式で first-fit 確保・分割・解放・隣接併合を実装し、外部断片化と、それを緩める coalescing を示す。速さに振り切った bump アロケータも対比する。

## 肝

- **free list**: 空きブロックの一覧を持ち、要求に足る最初の空きを貸す(first-fit)
- **分割**: 空きが大きすぎれば、必要分を切り、余りを空きとして残す
- **解放と併合(coalesce)**: 返却時に隣り合う空きを 1 つにまとめる。これで大きな確保が再びできる
- **外部断片化**: 確保・解放を繰り返すと、空きの総量は足りても細切れで、大きな塊が取れなくなる
- **bump アロケータ**: ポインタを進めるだけの確保。速いが個別解放できず、Reset で一括解放(アリーナ)

## 効果の固定(テスト)

- `TestFragmentation`: 総空き 60 でも連続は 30 までで、40 の確保に失敗(外部断片化)
- `TestCoalesce`: 隣り合う空きを解放で併合し、大きな確保ができるようになる
- `TestBumpAllocator`: 高速確保・個別解放不可・Reset で一括解放

## 使い方

```go
a := allocator.New(100)
off, ok := a.Alloc(30) // first-fit で確保
a.Free(off)            // 解放 + 隣接併合
a.LargestFree()        // 一度に確保できる最大
a.Fragmentation()      // 外部断片化の度合い

b := allocator.NewBump(100)
b.Alloc(40) // ポインタ前進のみ
b.Reset()   // 全部一度に解放
```

## 簡略化したこと

- **オフセットのみ**: 実バイト列でなく区画の位置とサイズだけを管理
- **first-fit のみ**: best-fit / segregated free list / buddy などの方式は扱わない
- **アライメントなし**: 実物は境界揃え(8/16 バイト)やヘッダを持つ
- **スレッド非対応**: 実物はロックや per-thread arena(tcmalloc/jemalloc)で並行確保を捌く

## 章

教科書: [メモリアロケータ](https://sharin-2a1.pages.dev/parts/allocator)

実行: `go test ./foundations/allocator/`
