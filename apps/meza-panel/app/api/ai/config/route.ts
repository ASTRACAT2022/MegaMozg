import { NextResponse } from "next/server";

import { coreJson } from "@/lib/server-core";

type UpdateAIConfigBody = {
  provider?: string;
  gemini_api_key?: string;
  clear_gemini_api_key?: boolean;
  gemini_model?: string;
  gemini_base_url?: string;
};

export async function GET() {
  try {
    const config = await coreJson("/api/v1/ai/config");
    return NextResponse.json(config);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to load ai config" },
      { status: 500 }
    );
  }
}

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as UpdateAIConfigBody;
    const updated = await coreJson("/api/v1/ai/config", {
      method: "POST",
      body: JSON.stringify(body),
    });
    return NextResponse.json(updated);
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "failed to update ai config" },
      { status: 500 }
    );
  }
}
