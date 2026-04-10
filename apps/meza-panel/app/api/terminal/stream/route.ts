import { NextResponse } from "next/server";

import { coreFetch } from "@/lib/server-core";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

export async function POST(request: Request) {
  try {
    const rawBody = await request.text();
    const response = await coreFetch("/api/v1/terminal/stream", {
      method: "POST",
      headers: {
        Accept: "text/event-stream",
        "Content-Type": "application/json",
      },
      body: rawBody,
    });

    if (!response.ok) {
      const text = await response.text();
      return NextResponse.json({ error: text || "failed to open terminal stream" }, { status: response.status });
    }

    return new Response(response.body, {
      status: 200,
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        Connection: "keep-alive",
      },
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "terminal stream failed" },
      { status: 500 }
    );
  }
}
