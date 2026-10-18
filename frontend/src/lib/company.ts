/** Nome da empresa onde o auditor está instalado. Vem de AUDITOR_COMPANY_NAME no .env. */
export function companyName(): string {
  const raw = import.meta.env.VITE_COMPANY_NAME;
  if (typeof raw === "string" && raw.trim()) {
    return raw.trim();
  }
  return "db-auditor";
}
