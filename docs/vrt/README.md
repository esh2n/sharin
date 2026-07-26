# VRT（見た目回帰テスト）

デモコンポーネント（`components/*Demo.vue`）を実ブラウザで描画し、基準画像（reference PNG）と
**ピクセル差分0** で比較する。見た目の意図しない変化（AI臭いデザインの再発・レイアウト崩れ）を
機械的に検知する。arekore `packages/ui/vrt` 方式を VitePress/Vue 向けに移植したもの。

## 使い方

```sh
# 比較する（CI・普段の検証）。差分があれば落ちて vrt/diffs/ に差分画像が出る
pnpm run vrt

# 基準画像を採り直す（意図した見た目変更のとき only）。目視レビューを経てコミット
pnpm run vrt:update
```

このMacは corepack pnpm が壊れているので、直接は次で実行する:

```sh
mise exec node@22.20.0 -- npx pnpm@9.15.0 run vrt
```

初回は playwright の chromium が要る: `... npx pnpm@9.15.0 exec playwright install chromium`

## 仕組み（4段）

1. **拾い方**: `import.meta.glob("../components/*Demo.vue")` で全デモを自動収集。
   デモを足せば自動で VRT の対象になり、登録漏れが起きない
2. **描き方**: vitest browser mode（playwright chromium, viewport 720px 固定）で
   `#vrt-stage`（幅688px = 本文幅）に Vue で mount。VitePress テーマ CSS（`--vp-c-*`）を
   `setup.ts` で注入。アニメ・キャレットは CSS で止めて決定化
3. **撮り方**: `#vrt-stage` を screenshot。light/dark を `html.dark` の切替で2通り撮る
4. **比べ方**: `reference/<Demo>--<variant>--<theme>--<platform>.png` と pixelmatch。
   差分0が条件。落ちたら `diffs/` に差分画像（git 管理外）

## variant（同じデモを別 props で使うもの）

本文で同じデモを異なる props で複数回使うものは、`vrt.test.ts` の `VRT_VARIANTS` に
実使用に合わせて列挙する（1 variant = 1 基準画像）。例: `IdGenDemo` は uuidv4/uuidv7/ulid/snowflake。

必須 prop を持つデモを `VRT_VARIANTS` に登録し忘れると、mount 時の Vue warn を捕まえて
**テストが落ちる**（登録漏れを仕組みで検知）。

## 決定化（アニメするデモ）

mount 時に自動で状態が変わるデモ（現状 `RaftDemo` のみ）は `vrt` prop で自動 tick を止め、
決まった回数だけ進めた静止画を撮る。`RaftDemo` は seed 固定 LCG なので tick 数さえ固定すれば
完全再現できる。他のデモは初期状態が静的（乱数はボタン操作時のみ）なので props 不要。

## platform について

フォント描画がOSで変わるため、基準画像のファイル名に platform（`darwin` / `linux`）を含める。
現在の基準は mac（`darwin`）で採ったもの。CI を Docker/Linux で回す場合は同環境で `linux` の
基準を別途採る（描画を固定して差分0を成立させるため）。
