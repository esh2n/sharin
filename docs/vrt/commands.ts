import fs from "node:fs";
import path from "node:path";
import pixelmatch from "pixelmatch";
import { PNG } from "pngjs";
import type { BrowserCommand } from "vitest/node";

// NOTE: reference は「意図した見た目」の正本。更新は UPDATE_VRT=1 の明示実行に限る。
//       フォント描画がプラットフォームで変わるので、ファイル名に platform を含めて分離する
//       (mac で採った基準を Docker/Linux で誤って使わないため)。
const referencePath = (root: string, demo: string, variant: string, theme: string): string =>
  path.join(root, "vrt/reference", `${demo}--${variant}--${theme}--${process.platform}.png`);

const diffPath = (root: string, demo: string, variant: string, theme: string): string =>
  path.join(root, "vrt/diffs", `${demo}--${variant}--${theme}--${process.platform}.png`);

// NOTE: 同一環境なら描画は決定的なはずなので、差分ピクセルは 0 を要求する。
//       緩めたくなったら環境差(フォント・DPR)を先に疑う。
export const vrtCompare: BrowserCommand<[demo: string, variant: string, theme: string]> = async (
  ctx,
  demo,
  variant,
  theme,
) => {
  const root = ctx.project.config.root;
  const reference = referencePath(root, demo, variant, theme);
  const shot = await ctx.iframe.locator("#vrt-stage").screenshot({ animations: "disabled" });

  if (process.env.UPDATE_VRT === "1") {
    fs.mkdirSync(path.dirname(reference), { recursive: true });
    fs.writeFileSync(reference, shot);
    return;
  }

  if (!fs.existsSync(reference)) {
    throw new Error(
      `reference がありません: ${path.relative(root, reference)}\n` +
        "生成する: UPDATE_VRT=1 で VRT を実行して基準画像を採る",
    );
  }

  const expected = PNG.sync.read(fs.readFileSync(reference));
  const actual = PNG.sync.read(shot);
  if (expected.width !== actual.width || expected.height !== actual.height) {
    throw new Error(
      `サイズが reference と一致しません: ${expected.width}x${expected.height} (reference) ` +
        `vs ${actual.width}x${actual.height} (${demo}/${variant}/${theme})`,
    );
  }

  const diff = new PNG({ width: expected.width, height: expected.height });
  const diffPixels = pixelmatch(
    expected.data,
    actual.data,
    diff.data,
    expected.width,
    expected.height,
    { threshold: 0.1 },
  );
  if (diffPixels > 0) {
    const out = diffPath(root, demo, variant, theme);
    fs.mkdirSync(path.dirname(out), { recursive: true });
    fs.writeFileSync(out, PNG.sync.write(diff));
    throw new Error(
      `reference と ${diffPixels}px 差分があります (${demo}/${variant}/${theme})。diff: ${path.relative(root, out)}\n` +
        "意図した変更なら UPDATE_VRT=1 で基準を採り直し、目視レビューを経てコミットする",
    );
  }
};
