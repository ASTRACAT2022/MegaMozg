import { NextResponse } from "next/server";

import type { AIPlanResult, JobItem } from "@/lib/api";
import { summarizeJob } from "@/lib/alerts";
import { coreJson } from "@/lib/server-core";

type ChatRequestBody = {
  session_id?: string;
  message: string;
  auto_execute?: boolean;
};

type PlanAndCreateResponse = {
  plan: AIPlanResult;
  job: JobItem;
};

type AIConfigResponse = {
  provider: string;
  has_gemini_api_key: boolean;
};

type ChatOnlyResponse = {
  assistant_message: string;
};

type CoreAIChatResponse = {
  provider: string;
  message: string;
};

function isConversationalPrompt(input: string) {
  const normalized = input.trim().toLowerCase();
  if (!normalized) return false;

  const chatPatterns = [
    "hi",
    "hello",
    "привет",
    "здравствуй",
    "что ты можешь",
    "кто ты",
    "help",
    "помощь",
  ];

  const operationPatterns = [
    "обнов",
    "перезап",
    "restart",
    "update",
    "docker",
    "apt",
    "sudo",
    "systemctl",
    "service",
    "на ноде",
    "node:",
    "tag:",
    "fleet",
  ];

  if (operationPatterns.some((pattern) => normalized.includes(pattern))) {
    return false;
  }
  if (normalized.endsWith("?") || normalized.endsWith("？")) {
    return true;
  }
  if (normalized.split(/\s+/).length <= 4 && !operationPatterns.some((pattern) => normalized.includes(pattern))) {
    return true;
  }
  return chatPatterns.some((pattern) => normalized.includes(pattern));
}

function buildChatOnlyReply(config: AIConfigResponse): ChatOnlyResponse {
  if (config.provider === "gemini" && config.has_gemini_api_key) {
    return {
      assistant_message:
        "Я могу создавать и запускать задачи на нодах: обновление Docker, apt refresh, restart сервисов, rolling rollout. Напиши цель в формате: «обнови docker на ноде astra-1».",
    };
  }

  return {
    assistant_message:
      "Сейчас AI работает в fallback режиме. Вкладка «AI Агент» -> «Настройки AI (Gemini)»: укажи API key и сохрани. После этого смогу полноценно планировать задачи.",
  };
}

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as ChatRequestBody;
    if (!body.message || !body.message.trim()) {
      return NextResponse.json({ error: "message is required" }, { status: 400 });
    }

    const aiConfig = await coreJson<AIConfigResponse>("/api/v1/ai/config");
    if (isConversationalPrompt(body.message)) {
      try {
        const chatReply = await coreJson<CoreAIChatResponse>("/api/v1/ai/chat", {
          method: "POST",
          body: JSON.stringify({ prompt: body.message }),
        });
        return NextResponse.json({
          session_id: body.session_id ?? "default",
          plan: null,
          job: null,
          executed: false,
          assistant_message: chatReply.message,
        });
      } catch {
        return NextResponse.json({
          session_id: body.session_id ?? "default",
          plan: null,
          job: null,
          executed: false,
          assistant_message: buildChatOnlyReply(aiConfig).assistant_message,
        });
      }
    }

    const planResult = await coreJson<PlanAndCreateResponse>("/api/v1/ai/plan-and-create", {
      method: "POST",
      body: JSON.stringify({ prompt: body.message }),
    });

    let job = planResult.job;
    let executed = false;
    let note = "";

    if (body.auto_execute) {
      if (job.requires_approval) {
        job = await coreJson<JobItem>(`/api/v1/jobs/${job.id}/approve`, {
          method: "POST",
          body: JSON.stringify({ actor: "ai-copilot" }),
        });
      }
      job = await coreJson<JobItem>(`/api/v1/jobs/${job.id}/start`, {
        method: "POST",
        body: JSON.stringify({ actor: "ai-copilot" }),
      });
      executed = true;
      note = "Задача была автоматически подтверждена и запущена.";
    }

    const assistantMessage =
      `Понял. Сформировал задачу ${job.id}. ${summarizeJob(job)} ` +
      `${note || "При необходимости могу запустить после подтверждения."}`;

    return NextResponse.json({
      session_id: body.session_id ?? "default",
      plan: planResult.plan,
      job,
      executed,
      assistant_message: assistantMessage,
    });
  } catch (error) {
    return NextResponse.json(
      { error: error instanceof Error ? error.message : "agent request failed" },
      { status: 500 }
    );
  }
}
