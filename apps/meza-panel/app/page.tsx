import { ControlCenter } from "@/components/control-center";
import { loadPanelData } from "@/lib/api";
import { buildAlerts } from "@/lib/alerts";

export default async function HomePage() {
  const panelData = await loadPanelData();
  const alerts = buildAlerts(panelData);

  return <ControlCenter initialState={{ ...panelData, alerts }} />;
}
