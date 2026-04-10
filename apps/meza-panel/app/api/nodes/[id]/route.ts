import { NextResponse } from "next/server";

import { coreJson } from "@/lib/server-core";

type UpdateNodeBody = {
  display_name: string;
};

export async function PATCH(request: Request, context: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await context.params;
    const body = (await request.json()) as UpdateNodeBody;
    const updated = await coreJson(`/api/v1/nodes/${id}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
    return NextResponse.json(updated);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update node" },
      { status: 500 }
    );
  }
}
