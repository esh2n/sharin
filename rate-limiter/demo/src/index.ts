import { DurableObject } from "cloudflare:workers";
import {
  ALGOS,
  type Algo,
  type AlgoState,
  CAPACITY,
  LIMIT,
  RATE_PER_SEC,
  WINDOW_MS,
  apply,
  isAlgo,
} from "./algorithms";

export interface Env {
  BUCKETS: DurableObjectNamespace<Bucket>;
}

// Bucket は「1つのキー(このデモでは algo + IP)の状態」を持つ Durable Object。
// 同じキーへのリクエストは世界中どこから来てもこの1インスタンスに直列化されるので、
// mutex も Lua も使わずに read-modify-write が原子的になる。
export class Bucket extends DurableObject {
  async take(algo: Algo): Promise<{ allowed: boolean; remaining: number; retryAfterMs: number }> {
    const state = await this.ctx.storage.get<AlgoState>("state");
    const r = apply(algo, state, Date.now());
    await this.ctx.storage.put("state", r.state);
    return { allowed: r.allowed, remaining: r.remaining, retryAfterMs: r.retryAfterMs };
  }
}

const CORS_HEADERS = {
  "access-control-allow-origin": "*",
  "access-control-allow-methods": "GET, OPTIONS",
};

const PARAMS: Record<Algo, Record<string, number>> = {
  "token-bucket": { capacity: CAPACITY, refillPerSec: RATE_PER_SEC },
  "leaky-bucket": { capacity: CAPACITY, leakPerSec: RATE_PER_SEC },
  "fixed-window": { limit: LIMIT, windowMs: WINDOW_MS },
  "sliding-window-log": { limit: LIMIT, windowMs: WINDOW_MS },
};

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    if (request.method === "OPTIONS") {
      return new Response(null, { headers: CORS_HEADERS });
    }

    const url = new URL(request.url);
    const algo = url.searchParams.get("algo") ?? "token-bucket";
    if (url.pathname !== "/check" || !isAlgo(algo)) {
      return Response.json(
        {
          usage: `GET /check?algo=<${ALGOS.join(" | ")}> を叩くと IP ごとに判定します`,
          source: "https://github.com/esh2n/sharin/tree/main/rate-limiter/demo",
        },
        { status: 404, headers: CORS_HEADERS },
      );
    }

    const ip = request.headers.get("cf-connecting-ip") ?? "unknown";
    const bucket = env.BUCKETS.get(env.BUCKETS.idFromName(`${algo}:${ip}`));
    const result = await bucket.take(algo);

    const headers: Record<string, string> = { ...CORS_HEADERS };
    if (!result.allowed) {
      headers["retry-after"] = String(Math.ceil(result.retryAfterMs / 1000));
    }
    return Response.json(
      {
        algo,
        allowed: result.allowed,
        remaining: result.remaining,
        retryAfterMs: result.retryAfterMs,
        params: PARAMS[algo],
        scope: "per-ip",
      },
      { status: result.allowed ? 200 : 429, headers },
    );
  },
} satisfies ExportedHandler<Env>;
