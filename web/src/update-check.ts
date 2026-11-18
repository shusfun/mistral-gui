export interface UpdateCheckView {
  available: boolean;
  current?: string;
  manifest?: { version: string; notes?: string };
}
function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}
export function parseUpdateCheck(value: unknown): UpdateCheckView {
  if (!record(value) || typeof value.available !== "boolean") throw new Error("update_response_invalid");
  const current = typeof value.current === "string" ? value.current : undefined;
  if (!value.available) return { available: false, current };
  const manifest = value.manifest;
  if (!record(manifest) || typeof manifest.version !== "string" || !/^v?\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(manifest.version)) throw new Error("update_response_invalid");
  return { available: true, current, manifest: { version: manifest.version, notes: typeof manifest.notes === "string" ? manifest.notes : undefined } };
}
