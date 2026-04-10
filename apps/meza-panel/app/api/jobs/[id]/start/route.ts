import { NextResponse } from "next/server";

import { coreJson } from "@/lib/server-core";

export async function POST(_: Request, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    const started = await coreJson(`/api/v1/jobs/${id}/start`, {
      method: "POST",
      body: JSON.stringify({ actor: "panel-operator" }),
    });
    return NextResponse.json(started);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to start job" },
      { status: 500 }
    );
  }
}

