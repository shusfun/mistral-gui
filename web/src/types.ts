export type RouteId =
  | "overview"
  | "translate"
  | "instances"
  | "models"
  | "device"
  | "connection"
  | "activity"
  | "updates";

export interface FailureRecord {
  code: string;
  detail?: string;
  exit_code?: number;
  log_tail?: string;
  time: string;
}

export interface RuntimeState {
  state: string;
  stage?: string;
  started_at?: string;
  estimated_ms?: number;
  cooldown_until?: string;
  failure_count?: number;
  last_failure?: FailureRecord;
  backend: string;
  model: string;
  revision: string;
  error?: string;
  detail?: string;
  updated_at: string;
  active: number;
  requests: number;
  degraded_cpu?: boolean;
}

export interface Tuning {
  context?: number;
  sequences?: number;
  gpu_layers?: number;
  gpu_memory_utilization?: number;
  kv_cache?: string;
  isq?: string;
  batch_tokens?: number;
}

export interface ManagerSettings {
  theme: "system" | "light" | "dark";
  language: "" | "zh-CN" | "en-US";
  backend: "auto" | "cpu" | "cuda" | "metal";
  model: string;
  model_version?: string;
  previous_model?: string;
  previous_model_version?: string;
  auto_start: boolean;
  idle_minutes: number;
  mirrors?: string[];
  engine: string;
  previous_engine?: string;
  engine_version?: string;
  allowed_extensions?: string[];
  max_running_deployments?: number;
  cloud_api_url?: string;
  cloud_model?: string;
  translate_routing?: "local-first" | "local-only" | "cloud-only";
  screenshot_hotkey?: string;
  launch_at_login?: boolean;
  cloud_image_confirmed?: boolean;
}

export interface DeploymentView {
  id: string;
  default?: boolean;
  model: string;
  model_version?: string;
  previous_model?: string;
  previous_model_version?: string;
  backend: string;
  auto_start: boolean;
  idle_minutes: number;
  tuning: Tuning;
  fingerprint: string;
  status: RuntimeState;
}

export interface PrepareRequest {
  id?: string;
  model: string;
  model_version?: string;
  repository?: string;
  backend?: string;
  recommend?: boolean;
  start?: boolean;
  restart?: boolean;
  auto_start?: boolean;
  idle_minutes?: number;
  tuning?: Tuning;
}

export interface OperationItem {
  id: string;
  kind: string;
  state: "running" | "success" | "failed" | "cancelled" | "interrupted";
  progress: number;
  total: number;
  step: string;
  error?: string;
  detail?: string;
  asset_kind?: string;
  asset_id?: string;
  repository?: string;
  source?: string;
  deployment_id?: string;
  prepare?: PrepareRequest;
  trace_id: string;
  updated_at: string;
}

export interface CatalogAsset {
  id: string;
  name?: string;
  kind: string;
  version: string;
  backend?: string;
  format?: string;
  source?: string;
  url?: string;
  size: number;
  sha256: string;
  recommended?: boolean;
  repository?: string;
  path?: string;
}

export interface TempStorageReport {
  files: number;
  bytes: number;
}

export interface DictionarySyncStatus {
  revisions: number;
  pack_entries: number;
  packs: number;
  synced_at?: string;
}

export interface OCRStatus {
  installed: boolean;
  running: boolean;
  state: string;
  download_bytes: number;
  disk_bytes: number;
  engine: string;
  license: string;
  platform_capture: boolean;
}

export interface StatusResponse {
  status: RuntimeState;
  settings: ManagerSettings;
  has_cloud_key: boolean;
  recovered_from_backup: boolean;
  evicted_deployments?: string[];
  temp_storage: TempStorageReport;
  dictionary_sync: DictionarySyncStatus;
  ocr: OCRStatus;
  operations: OperationItem[];
  platform: string;
  version: string;
  model_revision: string;
  deployment_revision: string;
  deployments: DeploymentView[];
  endpoint: string;
  selected_backend: string;
  update_result?: Record<string, unknown>;
}

export interface GPUDetail {
  name: string;
  driver: string;
  total_bytes: number;
  available_bytes: number;
}

export interface DeviceInfoResponse {
  os: string;
  arch: string;
  cpu: boolean;
  metal: boolean;
  cuda: boolean;
  cuda_reason?: string;
  vc_runtime?: boolean;
  cpu_cores: number;
  memory: string;
  memory_available?: string;
  memory_bytes?: number;
  available_bytes?: number;
  gpus: string[];
  gpu_details?: GPUDetail[];
}

export interface RecommendationResponse {
  device: {
    os: string;
    arch: string;
    cpu_cores: number;
    memory_total_bytes: number;
    memory_available_bytes: number;
    unified_memory: boolean;
    metal: boolean;
    cuda: boolean;
    gpus: GPUDetail[];
  };
  backend: "cpu" | "cuda" | "metal";
  tuning: Tuning;
  estimated_bytes: number;
  estimated_loading_bytes: number;
  warnings: string[];
  advisory: boolean;
}

export interface CapabilitiesResponse {
  engine: string;
  features: Partial<Record<keyof Tuning, boolean>>;
}

export interface TranslateSegment {
  source: string;
  raw_source?: string;
  box?: [number, number, number, number];
  translation?: string;
  channel?: string;
  error?: string;
}

export interface DesktopTranslateResponse {
  segments: TranslateSegment[];
  ocr_text?: string;
  ocr_raw_text?: string;
  ocr_ms?: number;
  fallback_reason?: string;
  model_used?: string;
}
