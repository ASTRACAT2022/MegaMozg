import { AlertTriangle, CheckCircle2, Clock3, PauseCircle } from "lucide-react";

import { Badge } from "@/components/ui/badge";

const statusMap = {
  online: { variant: "secondary" as const, icon: CheckCircle2, label: "online" },
  degraded: { variant: "outline" as const, icon: AlertTriangle, label: "degraded" },
  approved: { variant: "secondary" as const, icon: CheckCircle2, label: "approved" },
  completed: { variant: "secondary" as const, icon: CheckCircle2, label: "completed" },
  running: { variant: "default" as const, icon: Clock3, label: "running" },
  awaiting_approval: { variant: "outline" as const, icon: PauseCircle, label: "awaiting approval" },
  paused: { variant: "outline" as const, icon: PauseCircle, label: "paused" },
} as const;

export function StatusBadge({ status }: { status: string }) {
  const config = statusMap[status as keyof typeof statusMap] ?? {
    variant: "outline" as const,
    icon: Clock3,
    label: status.replaceAll("_", " "),
  };

  const Icon = config.icon;

  return (
    <Badge variant={config.variant} className="gap-1.5 font-medium capitalize">
      <Icon className="size-3.5" />
      {config.label}
    </Badge>
  );
}
