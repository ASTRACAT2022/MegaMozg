"use client";

import { useMemo, useState } from "react";
import {
  AlertTriangle,
  Bot,
  CheckCircle2,
  Cpu,
  HardDrive,
  Network,
  Play,
  RefreshCcw,
  Send,
  Server,
  ShieldCheck,
  Sparkles,
  Terminal,
  Trash2,
} from "lucide-react";

import { StatusBadge } from "@/components/status-badge";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import type { AIConfig, AIPlanResult, JobItem, NodeItem, PanelData } from "@/lib/api";
import type { AlertItem } from "@/lib/alerts";

type PanelState = PanelData & { alerts: AlertItem[] };

type ChatReply = {
  session_id: string;
  plan: AIPlanResult;
  job: JobItem;
  executed: boolean;
  assistant_message: string;
};

type ChatMessage = {
  id: string;
  role: "user" | "assistant" | "system";
  text: string;
  plan?: AIPlanResult;
  job?: JobItem;
};

type JobFormState = {
  type: "shell_command" | "bash_script";
  target_selector: string;
  strategy: string;
  execution_body: string;
  created_by: string;
  summary: string;
};

type TerminalHistoryItem = {
  id: string;
  node: string;
  command: string;
  status: string;
  output: string;
  createdAt: string;
  jobId?: string;
};

type TerminalExecMode = "job_simulated" | "local_exec";

type AIConfigFormState = {
  provider: "stub" | "gemini";
  gemini_api_key: string;
  gemini_model: string;
  gemini_base_url: string;
};

const initialJobForm: JobFormState = {
  type: "bash_script",
  target_selector: "node:argentina-17",
  strategy: "rolling:10,25,50,100",
  execution_body: "sudo apt update && sudo apt upgrade -y",
  created_by: "panel-operator",
  summary: "Обновление пакетов через bash",
};

function formatTime(value: string) {
  return new Intl.DateTimeFormat("ru-RU", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function metricTone(value: number) {
  if (value >= 80) return "text-destructive";
  if (value >= 60) return "text-amber-600";
  return "text-foreground";
}

function alertVariant(level: AlertItem["level"]) {
  if (level === "critical") return "destructive" as const;
  if (level === "warning") return "outline" as const;
  return "secondary" as const;
}

async function requestJson<T>(path: string, init: RequestInit = {}) {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(path, {
    ...init,
    headers,
    cache: "no-store",
  });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(text || `Ошибка запроса: ${response.status}`);
  }
  if (!text) return {} as T;
  return JSON.parse(text) as T;
}

function summaryCards(state: PanelState) {
  return [
    {
      title: "Онлайн-ноды",
      value: String(state.dashboard.online_nodes),
      hint: `Всего: ${state.dashboard.total_nodes}`,
      icon: Server,
    },
    {
      title: "Задачи в работе",
      value: String(state.dashboard.running_jobs),
      hint: `Ожидают подтверждения: ${state.dashboard.awaiting_approvals}`,
      icon: Play,
    },
    {
      title: "Средний CPU",
      value: `${state.dashboard.average_cpu}%`,
      hint: "Средняя загрузка по инфраструктуре",
      icon: Cpu,
    },
    {
      title: "Средний диск",
      value: `${state.dashboard.average_disk}%`,
      hint: "Использование диска по инфраструктуре",
      icon: HardDrive,
    },
  ];
}

export function ControlCenter({ initialState }: { initialState: PanelState }) {
  const [state, setState] = useState(initialState);
  const [activeTab, setActiveTab] = useState("overview");
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [isMutating, setIsMutating] = useState(false);
  const [statusText, setStatusText] = useState<string>("");
  const [errorText, setErrorText] = useState<string>("");

  const [jobForm, setJobForm] = useState<JobFormState>(initialJobForm);

  const [chatInput, setChatInput] = useState("");
  const [autoExecute, setAutoExecute] = useState(false);
  const [chatSession] = useState("default");
  const [isChatSending, setIsChatSending] = useState(false);
  const [chatMessages, setChatMessages] = useState<ChatMessage[]>([
    {
      id: "hello",
      role: "assistant",
      text: "Привет. Напиши задачу обычным языком, например: «обнови docker на ноде argentina-17». Я создам типизированный job и предложу запуск.",
    },
  ]);
  const [terminalNode, setTerminalNode] = useState(initialState.nodes[0]?.name ?? "argentina-17");
  const [terminalCommand, setTerminalCommand] = useState("sudo systemctl status docker");
  const [terminalMode, setTerminalMode] = useState<TerminalExecMode>("job_simulated");
  const [terminalHistory, setTerminalHistory] = useState<TerminalHistoryItem[]>([]);
  const [isTerminalRunning, setIsTerminalRunning] = useState(false);
  const [terminalLiveOutput, setTerminalLiveOutput] = useState<string>("");
  const [nodeDisplayNameDrafts, setNodeDisplayNameDrafts] = useState<Record<string, string>>(() =>
    Object.fromEntries(initialState.nodes.map((node) => [node.id, node.display_name || ""]))
  );
  const [aiConfigForm, setAIConfigForm] = useState<AIConfigFormState>({
    provider: initialState.aiConfig.provider === "gemini" ? "gemini" : "stub",
    gemini_api_key: "",
    gemini_model: initialState.aiConfig.gemini_model || "gemini-2.5-flash",
    gemini_base_url: initialState.aiConfig.gemini_base_url || "https://generativelanguage.googleapis.com/v1beta",
  });
  const [clearGeminiKey, setClearGeminiKey] = useState(false);

  const cards = useMemo(() => summaryCards(state), [state]);

  async function refreshState() {
    setIsRefreshing(true);
    try {
      const nextState = await requestJson<PanelState>("/api/state");
      setState(nextState);
      setNodeDisplayNameDrafts(Object.fromEntries(nextState.nodes.map((node) => [node.id, node.display_name || ""])));
      setAIConfigForm((prev) => ({
        ...prev,
        provider: nextState.aiConfig.provider === "gemini" ? "gemini" : "stub",
        gemini_model: nextState.aiConfig.gemini_model || "gemini-2.5-flash",
        gemini_base_url: nextState.aiConfig.gemini_base_url || "https://generativelanguage.googleapis.com/v1beta",
      }));
      setErrorText("");
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : "Не удалось обновить состояние.");
    } finally {
      setIsRefreshing(false);
    }
  }

  function patchJobForm<K extends keyof JobFormState>(key: K, value: JobFormState[K]) {
    setJobForm((prev) => ({ ...prev, [key]: value }));
  }

  function patchNodeDisplayNameDraft(nodeId: string, value: string) {
    setNodeDisplayNameDrafts((prev) => ({
      ...prev,
      [nodeId]: value,
    }));
  }

  async function saveNodeDisplayName(nodeId: string) {
    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      const displayName = (nodeDisplayNameDrafts[nodeId] ?? "").trim();
      const updated = await requestJson<NodeItem>(`/api/nodes/${nodeId}`, {
        method: "PATCH",
        body: JSON.stringify({ display_name: displayName }),
      });
      setStatusText(
        displayName
          ? `Имя ноды ${updated.name} обновлено на «${updated.display_name}».`
          : `Пользовательское имя для ${updated.name} очищено.`
      );
      await refreshState();
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : "Не удалось обновить имя ноды.");
    } finally {
      setIsMutating(false);
    }
  }

  async function createJob() {
    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      const body = jobForm.execution_body.trim();
      if (!body) {
        throw new Error("Поле команды/скрипта не может быть пустым.");
      }
      const payload: Record<string, unknown> =
        jobForm.type === "shell_command" ? { command: body } : { script: body };
      const created = await requestJson<JobItem>("/api/jobs", {
        method: "POST",
        body: JSON.stringify({
          type: jobForm.type.trim(),
          target_selector: jobForm.target_selector.trim(),
          strategy: jobForm.strategy.trim(),
          payload,
          created_by: jobForm.created_by.trim(),
          summary: jobForm.summary.trim(),
        }),
      });
      setStatusText(`Задача ${created.id} успешно создана.`);
      await refreshState();
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : "Не удалось создать задачу.");
    } finally {
      setIsMutating(false);
    }
  }

  async function approveJob(id: string) {
    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      await requestJson<JobItem>(`/api/jobs/${id}/approve`, { method: "POST" });
      setStatusText(`Задача ${id} подтверждена.`);
      await refreshState();
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : `Не удалось подтвердить задачу ${id}.`);
    } finally {
      setIsMutating(false);
    }
  }

  async function startJob(id: string) {
    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      await requestJson<JobItem>(`/api/jobs/${id}/start`, { method: "POST" });
      setStatusText(`Задача ${id} запущена.`);
      await refreshState();
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : `Не удалось запустить задачу ${id}.`);
    } finally {
      setIsMutating(false);
    }
  }

  async function deleteJob(id: string) {
    const shouldDelete = window.confirm(`Удалить задачу ${id}? Это действие нельзя отменить.`);
    if (!shouldDelete) {
      return;
    }

    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      await requestJson<JobItem>(`/api/jobs/${id}`, { method: "DELETE" });
      setStatusText(`Задача ${id} удалена.`);
      await refreshState();
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : `Не удалось удалить задачу ${id}.`);
    } finally {
      setIsMutating(false);
    }
  }

  async function sendAgentMessage() {
    const content = chatInput.trim();
    if (!content) return;
    if (state.aiConfig.provider === "gemini" && !state.aiConfig.has_gemini_api_key) {
      setErrorText("Gemini выбран, но API ключ не задан. Добавьте ключ в настройках AI.");
      setActiveTab("agent");
      return;
    }

    setIsChatSending(true);
    setErrorText("");
    const userMessage: ChatMessage = {
      id: `${Date.now()}-u`,
      role: "user",
      text: content,
    };
    setChatMessages((prev) => [...prev, userMessage]);
    setChatInput("");

    try {
      const reply = await requestJson<ChatReply>("/api/agent/chat", {
        method: "POST",
        body: JSON.stringify({
          session_id: chatSession,
          message: content,
          auto_execute: autoExecute,
        }),
      });
      setChatMessages((prev) => [
        ...prev,
        {
          id: `${Date.now()}-a`,
          role: "assistant",
          text: reply.assistant_message,
          plan: reply.plan,
          job: reply.job,
        },
      ]);
      setStatusText(reply.executed ? `AI-агент создал и запустил ${reply.job.id}.` : `AI-агент создал ${reply.job.id}.`);
      await refreshState();
      setActiveTab("agent");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Ошибка AI-агента.";
      setChatMessages((prev) => [
        ...prev,
        {
          id: `${Date.now()}-s`,
          role: "system",
          text: `Ошибка: ${message}`,
        },
      ]);
      setErrorText(message);
    } finally {
      setIsChatSending(false);
    }
  }

  async function saveAIConfig() {
    setIsMutating(true);
    setStatusText("");
    setErrorText("");
    try {
      const provider = aiConfigForm.provider === "gemini" ? "gemini" : "stub";
      const model = aiConfigForm.gemini_model.trim();
      const baseURL = aiConfigForm.gemini_base_url.trim();
      const key = aiConfigForm.gemini_api_key.trim();

      if (provider === "gemini" && !key && !clearGeminiKey && !state.aiConfig.has_gemini_api_key) {
        throw new Error("Укажи Gemini API key, иначе агент не сможет выполнять запросы.");
      }

      const body: Record<string, unknown> = {
        provider,
        gemini_model: model || "gemini-2.5-flash",
        gemini_base_url: baseURL || "https://generativelanguage.googleapis.com/v1beta",
      };
      if (key) {
        body.gemini_api_key = key;
      }
      if (clearGeminiKey) {
        body.clear_gemini_api_key = true;
      }

      const updated = await requestJson<AIConfig>("/api/ai/config", {
        method: "POST",
        body: JSON.stringify(body),
      });

      setState((prev) => ({
        ...prev,
        aiConfig: updated,
      }));
      setAIConfigForm((prev) => ({
        ...prev,
        provider: updated.provider === "gemini" ? "gemini" : "stub",
        gemini_model: updated.gemini_model || "gemini-2.5-flash",
        gemini_base_url: updated.gemini_base_url || "https://generativelanguage.googleapis.com/v1beta",
        gemini_api_key: "",
      }));
      setClearGeminiKey(false);
      setStatusText(`AI настройки сохранены. Провайдер: ${updated.provider}.`);
    } catch (error) {
      setErrorText(error instanceof Error ? error.message : "Не удалось сохранить AI настройки.");
    } finally {
      setIsMutating(false);
    }
  }

  async function runTerminalCommand() {
    const command = terminalCommand.trim();
    if (!command) {
      setErrorText("Введите команду для выполнения.");
      return;
    }

    setIsTerminalRunning(true);
    setStatusText("");
    setErrorText("");
    setTerminalLiveOutput("");

    try {
      const response = await fetch("/api/terminal/stream", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          node_name: terminalNode,
          command,
          mode: terminalMode,
        }),
      });

      if (!response.ok) {
        const text = await response.text();
        throw new Error(text || "Не удалось открыть поток терминала.");
      }
      if (!response.body) {
        throw new Error("Поток терминала не вернул тело ответа.");
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      let finalStatus = "running";
      let jobId = "";
      let aggregatedOutput = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const normalized = buffer.replaceAll("\r\n", "\n");
        const chunks = normalized.split("\n\n");
        buffer = chunks.pop() ?? "";

        for (const chunk of chunks) {
          if (!chunk.trim()) continue;
          const lines = chunk.split("\n");
          let eventName = "";
          let dataText = "";

          for (const line of lines) {
            if (line.startsWith("event:")) {
              eventName = line.slice(6).trim();
            } else if (line.startsWith("data:")) {
              dataText += line.slice(5).trim();
            }
          }

          if (!dataText) continue;

          try {
            const payload = JSON.parse(dataText) as { line?: string; status?: string; job_id?: string; error?: string };
            if (payload.job_id) {
              jobId = payload.job_id;
            }
            if (payload.status) {
              finalStatus = payload.status;
            }
            if (payload.line) {
              aggregatedOutput += `${payload.line}\n`;
              setTerminalLiveOutput((prev) => `${prev}${payload.line}\n`);
            }
            if (eventName === "error") {
              throw new Error(payload.error || "Ошибка потока терминала.");
            }
          } catch (error) {
            if (eventName === "error") {
              throw error;
            }
            const line = `[parse-error] ${error instanceof Error ? error.message : "bad event"}\n`;
            aggregatedOutput += line;
            setTerminalLiveOutput((prev) => `${prev}${line}`);
          }
        }
      }

      const entry: TerminalHistoryItem = {
        id: `${Date.now()}`,
        node: terminalNode,
        command,
        status: finalStatus,
        output: aggregatedOutput.trim() || "Команда завершена.",
        createdAt: new Date().toISOString(),
        jobId: jobId || undefined,
      };
      setTerminalHistory((prev) => [entry, ...prev].slice(0, 25));
      setStatusText(`Терминальная сессия завершена. Job: ${jobId || "n/a"} (${finalStatus}).`);
      await refreshState();
      setActiveTab("terminal");
    } catch (error) {
      const message = error instanceof Error ? error.message : "Ошибка выполнения SSH-команды.";
      setErrorText(message);
      setTerminalHistory((prev) => [
        {
          id: `${Date.now()}`,
          node: terminalNode,
          command,
          status: "failed",
          output: `Ошибка: ${message}`,
          createdAt: new Date().toISOString(),
        },
        ...prev,
      ]);
    } finally {
      setIsTerminalRunning(false);
    }
  }

  return (
    <main className="min-h-screen">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-6 md:px-6 lg:px-8">
        <Card>
          <CardHeader className="gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="space-y-2">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="secondary" className="gap-1.5">
                    <ShieldCheck className="size-3.5" />
                    Shadcn UI
                  </Badge>
                  <Badge variant="outline" className="gap-1.5">
                    <Bot className="size-3.5" />
                    AI-оркестрация
                  </Badge>
                </div>
                <CardTitle className="text-2xl tracking-tight md:text-3xl">MezaMozg Control Center</CardTitle>
                <CardDescription>
                  Централизованное управление инфраструктурой: ноды, задачи, алерты и AI-агент в одном интерфейсе.
                </CardDescription>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <Button
                  variant="outline"
                  onClick={() => {
                    setActiveTab("alerts");
                    setStatusText("");
                  }}
                >
                  <AlertTriangle className="size-4" />
                  Алерты
                </Button>
                <Button
                  variant="outline"
                  onClick={() => {
                    setActiveTab("terminal");
                    setStatusText("Открыт ручной SSH-терминал.");
                  }}
                >
                  <Terminal className="size-4" />
                  SSH Терминал
                </Button>
                <Button
                  onClick={() => {
                    setActiveTab("jobs");
                    setStatusText("Открыта вкладка задач. Заполни форму и нажми «Создать задачу».");
                  }}
                >
                  Создать задачу
                </Button>
                <Button variant="ghost" onClick={refreshState} disabled={isRefreshing}>
                  <RefreshCcw className={`size-4 ${isRefreshing ? "animate-spin" : ""}`} />
                  Обновить
                </Button>
              </div>
            </div>

            {!state.apiReachable ? (
              <div className="rounded-lg border border-dashed bg-muted/50 p-3 text-sm">
                Backend временно недоступен, отображается демо-состояние. Проверь `MEZA_CORE_BASE_URL` и токен оператора.
              </div>
            ) : null}

            {statusText ? (
              <div className="rounded-lg border bg-emerald-50 p-3 text-sm text-emerald-700">{statusText}</div>
            ) : null}
            {errorText ? (
              <div className="rounded-lg border bg-red-50 p-3 text-sm text-red-700">{errorText}</div>
            ) : null}
          </CardHeader>
        </Card>

        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList className="h-auto w-full flex-wrap justify-start">
            <TabsTrigger value="overview">Обзор</TabsTrigger>
            <TabsTrigger value="nodes">Ноды</TabsTrigger>
            <TabsTrigger value="jobs">Задачи</TabsTrigger>
            <TabsTrigger value="terminal">SSH Терминал</TabsTrigger>
            <TabsTrigger value="agent">AI Агент</TabsTrigger>
            <TabsTrigger value="alerts">Алерты и аудит</TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-4">
            <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
              {cards.map((card) => {
                const Icon = card.icon;
                return (
                  <Card key={card.title}>
                    <CardHeader className="flex flex-row items-start justify-between space-y-0">
                      <div className="space-y-1">
                        <CardDescription>{card.title}</CardDescription>
                        <CardTitle className="text-3xl font-semibold tracking-tight">{card.value}</CardTitle>
                      </div>
                      <div className="rounded-lg border bg-muted p-2 text-muted-foreground">
                        <Icon className="size-4" />
                      </div>
                    </CardHeader>
                    <CardContent>
                      <p className="text-muted-foreground text-sm">{card.hint}</p>
                    </CardContent>
                  </Card>
                );
              })}
            </section>

            <section className="grid gap-4 lg:grid-cols-2">
              <Card>
                <CardHeader>
                  <CardTitle>Состояние инфраструктуры</CardTitle>
                  <CardDescription>Ключевые метрики для быстрого решения о rollout.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2 font-medium">
                        <Cpu className="size-4 text-muted-foreground" />
                        Средний CPU
                      </span>
                      <span>{state.dashboard.average_cpu}%</span>
                    </div>
                    <Progress value={state.dashboard.average_cpu} />
                  </div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2 font-medium">
                        <HardDrive className="size-4 text-muted-foreground" />
                        Средний RAM
                      </span>
                      <span>{state.dashboard.average_ram}%</span>
                    </div>
                    <Progress value={state.dashboard.average_ram} />
                  </div>
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span className="flex items-center gap-2 font-medium">
                        <Network className="size-4 text-muted-foreground" />
                        Суммарный network
                      </span>
                      <span>{state.nodes.reduce((sum, node) => sum + node.metrics.network_kbps, 0)} kbps</span>
                    </div>
                    <Progress value={Math.min(100, state.nodes.reduce((sum, node) => sum + node.metrics.network_kbps, 0) / 30)} />
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Что происходит сейчас</CardTitle>
                  <CardDescription>Быстрый контекст по очередям и рискам.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="rounded-lg border p-3">
                    <p className="text-muted-foreground text-sm">Ожидают подтверждения</p>
                    <p className="text-2xl font-semibold">{state.dashboard.awaiting_approvals}</p>
                  </div>
                  <div className="rounded-lg border p-3">
                    <p className="text-muted-foreground text-sm">Деградированные ноды</p>
                    <p className="text-2xl font-semibold">{state.dashboard.degraded_nodes}</p>
                  </div>
                  <div className="rounded-lg border p-3">
                    <p className="text-muted-foreground text-sm">Событий аудита</p>
                    <p className="text-2xl font-semibold">{state.dashboard.audit_events}</p>
                  </div>
                </CardContent>
              </Card>
            </section>
          </TabsContent>

          <TabsContent value="nodes">
            <Card>
              <CardHeader>
                <CardTitle>Ноды</CardTitle>
                <CardDescription>Инвентарь серверов с метриками и статусами.</CardDescription>
              </CardHeader>
              <CardContent>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Нода</TableHead>
                      <TableHead>IP</TableHead>
                      <TableHead>Имя в панели</TableHead>
                      <TableHead>Статус</TableHead>
                      <TableHead>Теги</TableHead>
                      <TableHead className="w-[180px]">CPU</TableHead>
                      <TableHead className="w-[180px]">RAM</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {state.nodes.map((node) => (
                      <TableRow key={node.id}>
                        <TableCell className="align-top">
                          <div className="space-y-1">
                            <div className="font-medium">{node.display_name || node.name}</div>
                            <div className="text-muted-foreground text-xs">Системное: {node.name}</div>
                            <div className="text-muted-foreground text-xs">
                              {node.region} · {formatTime(node.last_seen_at)}
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="font-mono text-sm">{node.ip_address || "—"}</div>
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="flex min-w-[220px] gap-2">
                            <Input
                              value={nodeDisplayNameDrafts[node.id] ?? ""}
                              onChange={(event) => patchNodeDisplayNameDraft(node.id, event.target.value)}
                              placeholder="Например: DB Москва"
                            />
                            <Button variant="outline" onClick={() => saveNodeDisplayName(node.id)} disabled={isMutating}>
                              Сохранить
                            </Button>
                          </div>
                        </TableCell>
                        <TableCell className="align-top">
                          <StatusBadge status={node.status} />
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="flex max-w-xs flex-wrap gap-1">
                            {node.tags.map((tag) => (
                              <Badge key={`${node.id}-${tag}`} variant="outline">
                                {tag}
                              </Badge>
                            ))}
                          </div>
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="space-y-2">
                            <div className={`text-sm font-medium ${metricTone(node.metrics.cpu_percent)}`}>
                              {node.metrics.cpu_percent}%
                            </div>
                            <Progress value={node.metrics.cpu_percent} />
                          </div>
                        </TableCell>
                        <TableCell className="align-top">
                          <div className="space-y-2">
                            <div className={`text-sm font-medium ${metricTone(node.metrics.ram_percent)}`}>
                              {node.metrics.ram_percent}%
                            </div>
                            <Progress value={node.metrics.ram_percent} />
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>
          </TabsContent>

          <TabsContent value="jobs">
            <section className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
              <Card>
                <CardHeader>
                  <CardTitle>Создать задачу</CardTitle>
                  <CardDescription>Форма создаёт реальную задачу в Meza-Core через `/api/jobs`.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Тип задачи</p>
                    <select
                      className="border-input bg-background ring-offset-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                      value={jobForm.type}
                      onChange={(event) => patchJobForm("type", event.target.value as JobFormState["type"])}
                    >
                      <option value="shell_command">shell_command</option>
                      <option value="bash_script">bash_script</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Цель</p>
                    <Input
                      value={jobForm.target_selector}
                      onChange={(event) => patchJobForm("target_selector", event.target.value)}
                      placeholder="node:argentina-17 или tag:frontend"
                    />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Стратегия rollout</p>
                    <Input
                      value={jobForm.strategy}
                      onChange={(event) => patchJobForm("strategy", event.target.value)}
                      placeholder="rolling:10,25,50,100"
                    />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Создал</p>
                    <Input value={jobForm.created_by} onChange={(event) => patchJobForm("created_by", event.target.value)} />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Краткое описание</p>
                    <Input value={jobForm.summary} onChange={(event) => patchJobForm("summary", event.target.value)} />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">
                      {jobForm.type === "shell_command" ? "Команда shell" : "Содержимое bash-скрипта"}
                    </p>
                    <Textarea
                      className="min-h-32 font-mono text-xs"
                      value={jobForm.execution_body}
                      onChange={(event) => patchJobForm("execution_body", event.target.value)}
                    />
                    <p className="text-muted-foreground text-xs">
                      В API уйдёт payload:
                      {jobForm.type === "shell_command" ? " { command: ... }" : " { script: ... }"}
                    </p>
                  </div>

                  <Button onClick={createJob} disabled={isMutating}>
                    Создать задачу
                  </Button>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Список задач</CardTitle>
                  <CardDescription>Подтверждение и запуск задач без перехода в консоль.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {state.jobs.map((job) => {
                    const canApprove = job.status === "awaiting_approval";
                    const canStart = job.status === "approved" || job.status === "planned";

                    return (
                      <div key={job.id} className="rounded-lg border p-4">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div className="space-y-1">
                            <div className="font-medium">
                              {job.id} · {job.type}
                            </div>
                            <p className="text-muted-foreground text-sm">{job.summary}</p>
                          </div>
                          <StatusBadge status={job.status} />
                        </div>

                        <div className="mt-3 grid gap-2 text-sm sm:grid-cols-2">
                          <div>
                            <p className="text-muted-foreground">Цель</p>
                            <p className="font-medium">{job.target_selector}</p>
                          </div>
                          <div>
                            <p className="text-muted-foreground">Стратегия</p>
                            <p className="font-medium">{job.strategy}</p>
                          </div>
                          <div>
                            <p className="text-muted-foreground">Создал</p>
                            <p className="font-medium">{job.created_by}</p>
                          </div>
                          <div>
                            <p className="text-muted-foreground">Нод в выборке</p>
                            <p className="font-medium">{job.matched_nodes.length}</p>
                          </div>
                        </div>

                        <div className="mt-3 flex flex-wrap gap-2">
                          <Badge variant={job.requires_approval ? "outline" : "secondary"}>
                            {job.requires_approval ? "Требует подтверждения" : "Можно запускать"}
                          </Badge>
                          {job.matched_nodes.slice(0, 3).map((node) => (
                            <Badge key={`${job.id}-${node}`} variant="outline">
                              {node}
                            </Badge>
                          ))}
                        </div>

                        <div className="mt-4 flex flex-wrap gap-2">
                          <Button variant="outline" onClick={() => approveJob(job.id)} disabled={!canApprove || isMutating}>
                            Подтвердить
                          </Button>
                          <Button onClick={() => startJob(job.id)} disabled={!canStart || isMutating}>
                            Запустить
                          </Button>
                          <Button variant="destructive" onClick={() => deleteJob(job.id)} disabled={isMutating}>
                            <Trash2 className="size-4" />
                            Удалить
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </CardContent>
              </Card>
            </section>
          </TabsContent>

          <TabsContent value="terminal">
            <section className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Terminal className="size-4 text-muted-foreground" />
                    Ручной SSH терминал
                  </CardTitle>
                  <CardDescription>
                    Команда отправляется как `shell_command` job на выбранную ноду.
                  </CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Нода</p>
                    <select
                      className="border-input bg-background ring-offset-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                      value={terminalNode}
                      onChange={(event) => setTerminalNode(event.target.value)}
                    >
                      {state.nodes.map((node) => (
                        <option key={node.id} value={node.name}>
                          {node.display_name || node.name} ({node.region})
                        </option>
                      ))}
                    </select>
                  </div>

                  <div className="space-y-1">
                    <p className="text-sm font-medium">Команда</p>
                    <Textarea
                      className="min-h-32 font-mono text-xs"
                      value={terminalCommand}
                      onChange={(event) => setTerminalCommand(event.target.value)}
                      placeholder="sudo docker ps -a"
                    />
                  </div>

                  <div className="space-y-1">
                    <p className="text-sm font-medium">Режим потока</p>
                    <select
                      className="border-input bg-background ring-offset-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                      value={terminalMode}
                      onChange={(event) => setTerminalMode(event.target.value as TerminalExecMode)}
                    >
                      <option value="job_simulated">job_simulated (безопасный, через orchestration)</option>
                      <option value="local_exec">local_exec (выполняет команду на meza-core хосте)</option>
                    </select>
                  </div>

                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <Button onClick={runTerminalCommand} disabled={isTerminalRunning || isMutating}>
                      <Terminal className="size-4" />
                      Выполнить
                    </Button>
                  </div>

                  <p className="text-muted-foreground text-xs">
                    Поток идёт в реальном времени через `/api/terminal/stream` (SSE).
                  </p>

                  <div className="space-y-1">
                    <p className="text-sm font-medium">Live output</p>
                    <pre className="max-h-52 overflow-auto rounded-md border bg-muted/30 p-3 text-xs whitespace-pre-wrap">
                      {terminalLiveOutput || "Ожидание запуска команды..."}
                    </pre>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>История команд</CardTitle>
                  <CardDescription>Последние команды и статус выполнения.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {terminalHistory.length === 0 ? (
                    <div className="rounded-lg border bg-muted/20 p-3 text-sm">
                      История пуста. Выполни первую команду.
                    </div>
                  ) : null}

                  {terminalHistory.map((item) => (
                    <div key={item.id} className="rounded-lg border p-3">
                      <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="font-medium">
                          {item.node} · {item.jobId ?? "n/a"}
                        </div>
                        <StatusBadge status={item.status} />
                      </div>
                      <p className="mt-2 rounded bg-muted/40 px-2 py-1 font-mono text-xs">{item.command}</p>
                      <pre className="mt-2 max-h-60 overflow-auto rounded bg-muted/20 p-2 font-mono text-xs whitespace-pre-wrap">
                        {item.output}
                      </pre>
                      <p className="text-muted-foreground mt-2 text-xs">{formatTime(item.createdAt)}</p>
                    </div>
                  ))}
                </CardContent>
              </Card>
            </section>
          </TabsContent>

          <TabsContent value="agent">
            <section className="grid gap-4 xl:grid-cols-[1.2fr_0.8fr]">
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <Sparkles className="size-4 text-muted-foreground" />
                    AI Агент (чат)
                  </CardTitle>
                  <CardDescription>Пиши задачу простым языком. Агент создаёт типизированный job и возвращает план.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4">
                  <div className="max-h-[460px] space-y-3 overflow-y-auto rounded-lg border bg-muted/30 p-3">
                    {chatMessages.map((message) => (
                      <div
                        key={message.id}
                        className={`rounded-lg border p-3 ${
                          message.role === "user"
                            ? "ml-10 border-primary/30 bg-primary/5"
                            : message.role === "assistant"
                              ? "mr-10 bg-background"
                              : "mr-10 border-destructive/30 bg-red-50"
                        }`}
                      >
                        <p className="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                          {message.role === "user" ? "Оператор" : message.role === "assistant" ? "AI Агент" : "Система"}
                        </p>
                        <p className="text-sm whitespace-pre-wrap">{message.text}</p>

                        {message.plan ? (
                          <div className="mt-2 rounded-md border bg-muted/40 p-2">
                            <pre className="text-xs leading-5 whitespace-pre-wrap">{JSON.stringify(message.plan, null, 2)}</pre>
                          </div>
                        ) : null}

                        {message.job ? (
                          <div className="mt-2">
                            <Badge variant="outline">Создано: {message.job.id}</Badge>
                          </div>
                        ) : null}
                      </div>
                    ))}
                  </div>

                  <div className="space-y-2">
                    <Textarea
                      value={chatInput}
                      onChange={(event) => setChatInput(event.target.value)}
                      placeholder="Например: обнови docker на ноде argentina-17 и запусти по rolling стратегии"
                      className="min-h-24"
                    />
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <label className="flex items-center gap-2 text-sm">
                        <input
                          type="checkbox"
                          className="h-4 w-4 rounded border-input"
                          checked={autoExecute}
                          onChange={(event) => setAutoExecute(event.target.checked)}
                        />
                        Автозапуск (если не требуется подтверждение)
                      </label>
                      <Button onClick={sendAgentMessage} disabled={isChatSending}>
                        <Send className="size-4" />
                        Отправить
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Настройки AI (Gemini)</CardTitle>
                  <CardDescription>Введи API ключ один раз, и агент будет работать через Gemini.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3 text-sm">
                  <div className="rounded-lg border p-3 space-y-2">
                    <div className="flex items-center justify-between gap-2">
                      <p className="font-medium">Текущий провайдер</p>
                      <Badge variant="outline">{state.aiConfig.provider}</Badge>
                    </div>
                    <p className="text-muted-foreground">
                      Ключ Gemini: {state.aiConfig.has_gemini_api_key ? "задан" : "не задан"}
                    </p>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Провайдер</p>
                    <select
                      className="border-input bg-background ring-offset-background focus-visible:ring-ring flex h-9 w-full rounded-md border px-3 py-1 text-sm shadow-xs transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-offset-2"
                      value={aiConfigForm.provider}
                      onChange={(event) =>
                        setAIConfigForm((prev) => ({
                          ...prev,
                          provider: event.target.value === "gemini" ? "gemini" : "stub",
                        }))
                      }
                    >
                      <option value="stub">stub (локальный fallback)</option>
                      <option value="gemini">gemini (реальный AI)</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Gemini API key</p>
                    <Input
                      type="password"
                      value={aiConfigForm.gemini_api_key}
                      onChange={(event) =>
                        setAIConfigForm((prev) => ({
                          ...prev,
                          gemini_api_key: event.target.value,
                        }))
                      }
                      placeholder="AIza..."
                    />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Gemini model</p>
                    <Input
                      value={aiConfigForm.gemini_model}
                      onChange={(event) =>
                        setAIConfigForm((prev) => ({
                          ...prev,
                          gemini_model: event.target.value,
                        }))
                      }
                      placeholder="gemini-2.5-flash"
                    />
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Gemini base URL</p>
                    <Input
                      value={aiConfigForm.gemini_base_url}
                      onChange={(event) =>
                        setAIConfigForm((prev) => ({
                          ...prev,
                          gemini_base_url: event.target.value,
                        }))
                      }
                      placeholder="https://generativelanguage.googleapis.com/v1beta"
                    />
                  </div>
                  <label className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      className="h-4 w-4 rounded border-input"
                      checked={clearGeminiKey}
                      onChange={(event) => setClearGeminiKey(event.target.checked)}
                    />
                    Очистить сохранённый API ключ Gemini
                  </label>
                  <Button onClick={saveAIConfig} disabled={isMutating}>
                    Сохранить AI настройки
                  </Button>
                  <div className="rounded-lg border p-3">
                    <p className="font-medium">Пример 1</p>
                    <p className="text-muted-foreground">«Обнови docker на ноде argentina-17»</p>
                  </div>
                  <div className="rounded-lg border p-3">
                    <p className="font-medium">Пример 2</p>
                    <p className="text-muted-foreground">«Перезапусти сервис nginx на tag:frontend с rolling 10,25,50,100»</p>
                  </div>
                  <div className="rounded-lg border p-3">
                    <p className="font-medium">Важно</p>
                    <p className="text-muted-foreground">
                      Для опасных операций система сохраняет режим approval-first: сначала ревью, потом запуск.
                    </p>
                  </div>
                </CardContent>
              </Card>
            </section>
          </TabsContent>

          <TabsContent value="alerts">
            <section className="grid gap-4 xl:grid-cols-[0.9fr_1.1fr]">
              <Card>
                <CardHeader>
                  <CardTitle>Алерты</CardTitle>
                  <CardDescription>Автоматические сигналы по рискам и очередям.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-3">
                  {state.alerts.length === 0 ? (
                    <div className="rounded-lg border bg-muted/20 p-3 text-sm">Критичных алертов нет.</div>
                  ) : null}
                  {state.alerts.map((alert) => (
                    <div key={alert.id} className="rounded-lg border p-3">
                      <div className="flex items-center justify-between gap-2">
                        <p className="font-medium">{alert.title}</p>
                        <Badge variant={alertVariant(alert.level)}>{alert.level}</Badge>
                      </div>
                      <p className="text-muted-foreground mt-1 text-sm">{alert.message}</p>
                      <p className="text-muted-foreground mt-2 text-xs">Источник: {alert.source}</p>
                    </div>
                  ))}
                </CardContent>
              </Card>

              <Card>
                <CardHeader>
                  <CardTitle>Аудит</CardTitle>
                  <CardDescription>Кто, когда и что выполнял в control-plane.</CardDescription>
                </CardHeader>
                <CardContent>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Действие</TableHead>
                        <TableHead>Кто</TableHead>
                        <TableHead>Ресурс</TableHead>
                        <TableHead>Время</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {state.audit.map((event) => (
                        <TableRow key={event.id}>
                          <TableCell>
                            <div className="space-y-1">
                              <div className="font-medium">{event.action}</div>
                              <div className="text-muted-foreground text-xs">{event.message}</div>
                            </div>
                          </TableCell>
                          <TableCell>{event.actor}</TableCell>
                          <TableCell className="text-muted-foreground">
                            {event.resource_type}:{event.resource_id}
                          </TableCell>
                          <TableCell className="text-muted-foreground">{formatTime(event.created_at)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </CardContent>
              </Card>
            </section>
          </TabsContent>
        </Tabs>

        <div className="text-muted-foreground flex items-center gap-2 text-xs">
          <CheckCircle2 className="size-3.5" />
          Панель подключена к API через серверные proxy-роуты (`/api/*`) и не раскрывает операторский токен в браузере.
        </div>
      </div>
    </main>
  );
}
