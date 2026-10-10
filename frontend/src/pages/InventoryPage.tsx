import { InventoryExplorer } from "./parts/InventoryExplorer";
import { InventoryModelProvider } from "./parts/useInventoryExplorer";

export function InventoryPage() {
  return (
    <InventoryModelProvider>
      <InventoryExplorer />
    </InventoryModelProvider>
  );
}
