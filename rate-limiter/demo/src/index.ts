import { DurableObject } from "cloudflare:workers";
import { type BucketState, CAPACITY, REFILL_PER_SEC, take } from "./bucket";

export interface Env {
  BUCKETS: DurableObjectNamespace<Bucket>;
}

// Bucket は「1つのキー(このデモではIP)の残量」を持つ Durable Object。
// 同じキーへのリクエストは世界中どこから来てもこの1インスタンスに直列化されるので、
// mutex も Lua も使わずに read-modify-write が原子的になる。
export class Bucket extends DurableObject {
  async take(): Promise<{ allowed: boolean; remaining: number; retryAfterMs: number }> {
    const state = await this.ctx.storage.get<BucketState>("state");
    const r = take(state, Date.now());
    await this.ctx.storage.put("state", r.state);
    return { allowed: r.allowed, remaining: r.remaining, retryAfterMs: r.retryAfterMs };
  }
}

const CORS_HEADERS = {
  "access-control-allow-origin": "*",
  "access-control-allow-methods": "GET, OPTIONS",
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method === "OPTIONS") {
      return new Response(null, { headers: CORS_HEADERS });
    }

    const url = new URL(request.url);
    if (url.pathname !== "/check") {
      return Response.json(
        {
          usage: "GET /check を叩くと IP ごとの token bucket (容量5、補充0.5個/秒) で判定します",
          source: "https://github.com/esh2n/sharin/tree/main/rate-limiter/demo",
        },
        { status: 404, headers: CORS_HEADERS },
      );
    }

    const ip = request.headers.get("cf-connecting-ip") ?? "unknown";
    const bucket = env.BUCKETS.get(env.BUCKETS.idFromName(ip));
    const result = await bucket.take();

    const headers: Record<string, string> = { ...CORS_HEADERS };
    if (!result.allowed) {
      headers["retry-after"] = String(Math.ceil(result.retryAfterMs / 1000));
    }
    return Response.json(
      {
        allowed: result.allowed,
        remaining: result.remaining,
        retryAfterMs: result.retryAfterMs,
        algorithm: "token bucket",
        capacity: CAPACITY,
        refillPerSec: REFILL_PER_SEC,
        scope: "per-ip",
      },
      { status: result.allowed ? 200 : 429, headers },
    );
  },
} satisfies ExportedHandler<Env>;
