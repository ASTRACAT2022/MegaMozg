import type { JobItem, NodeItem, PanelData } from "@/lib/api";

export type AlertLevel = "info" | "warning" | "critical";

export type AlertItem = {
  id: string;
  level: AlertLevel;
  title: string;
  message: string;
  source: string;
};

export function buildAlerts(data: Pick<PanelData, "dashboard" | "nodes" | "jobs">): AlertItem[] {
  const alerts: AlertItem[] = [];

  if (data.dashboard.degraded_nodes > 0) {
    alerts.push({
      id: "degraded-nodes",
      level: "warning",
      title: "Есть деградированные ноды",
      message: `Обнаружено ${data.dashboard.degraded_nodes} нод(ы) в статусе degraded.`,
      source: "nodes",
    });
  }

  if (data.dashboard.awaiting_approvals > 0) {
    alerts.push({
      id: "pending-approvals",
      level: "info",
      title: "Очередь согласования задач",
      message: `Ожидают подтверждения: ${data.dashboard.awaiting_approvals}.`,
      source: "jobs",
    });
  }

  const hotNodes = data.nodes.filter((node) => node.metrics.cpu_percent >= 85 || node.metrics.ram_percent >= 85);
  hotNodes.forEach((node) => {
    const nodeName = node.display_name || node.name;
    alerts.push({
      id: `hot-${node.id}`,
      level: "critical",
      title: `Высокая нагрузка: ${nodeName}`,
      message: `CPU ${node.metrics.cpu_percent}%, RAM ${node.metrics.ram_percent}%.`,
      source: "metrics",
    });
  });

  data.jobs
    .filter((job) => job.status === "paused")
    .forEach((job) => {
      alerts.push({
        id: `paused-${job.id}`,
        level: "warning",
        title: `Rollout остановлен: ${job.id}`,
        message: `${job.type} paused. Нужна проверка и ручное решение.`,
        source: "rollout",
      });
    });

  return alerts;
}

export function summarizeJob(job: JobItem) {
  const approvalHint = job.requires_approval ? "Требует подтверждения." : "Можно запускать автоматически.";
  return `${job.type} -> ${job.target_selector}. ${approvalHint}`;
}
