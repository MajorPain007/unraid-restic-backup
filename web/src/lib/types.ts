export interface Schedule {
  mode: 'off' | 'cron' | 'interval';
  cron: string;
  every: number;
  unit: 'minutes' | 'hours' | 'days';
  catch_up: boolean;
}

export interface Settings {
  data_dir: string;
  host: string;
  nice: number;
  io_priority: 'low' | 'idle' | 'normal';
  start_delay: number;
  parity_pause: boolean;
  history_days: number;
  show_in_menu: boolean;
  secrets: 'plain' | 'key' | 'master';
}

export interface Repo {
  id: string;
  name: string;
  type: 'sftp' | 'local' | 'rest';
  sftp: { user: string; host: string; port: number; path: string };
  local: { path: string };
  rest: { url: string; user: string; cacert: string };
  options: {
    compression: 'auto' | 'off' | 'fastest' | 'better' | 'max';
    limit_upload: number;
    limit_download: number;
    connections: number;
    extra_flags: string[];
  };
  maintenance: {
    prune: { enabled: boolean; schedule: Schedule; max_unused: string };
    check: { enabled: boolean; schedule: Schedule; read_data: 'none' | 'subset' | 'all'; subset: string };
  };
  unlock_stale: boolean;
  append_only: boolean;
  has_password?: boolean;
  has_rest_password?: boolean;
  ssh_public_key?: string;
}

export interface Retention {
  enabled: boolean;
  last: number;
  hourly: number;
  daily: number;
  weekly: number;
  monthly: number;
  yearly: number;
  within: string;
}

export interface Job {
  id: string;
  name: string;
  enabled: boolean;
  repo: string;
  sources: string[];
  excludes: string[];
  iexcludes: string[];
  exclude_caches: boolean;
  exclude_larger_than: string;
  one_file_system: boolean;
  tags: string[];
  schedule: Schedule;
  retention: Retention;
  consistency: {
    mode: 'none' | 'zfs' | 'zfs-stop' | 'stop';
    containers: string[];
    vms: string[];
    skip: string[];
    timeout: number;
  };
  hooks: { before: string; after: string };
  notify: { success: boolean; warning: boolean; error: boolean; healthcheck: string };
  advanced: { host: string; read_concurrency: number; skip_if_unchanged: boolean; extra_flags: string[] };
}

export interface Config {
  version: number;
  settings: Settings;
  repos: Repo[];
  jobs: Job[];
}

export type Status = 'running' | 'success' | 'warning' | 'error' | 'cancelled' | string;

export interface Op {
  id: string;
  kind: 'backup' | 'forget' | 'prune' | 'check' | 'restore' | string;
  job: string;
  job_name: string;
  repo: string;
  repo_name: string;
  trigger: 'manual' | 'schedule' | string;
  started: number;
  ended?: number;
  status: Status;
  error: string;
  log: string;
  phase?: string;
  progress?: Record<string, any> | null;
  summary?: Record<string, any> | null;
  [key: string]: any;
}

export interface LastRun {
  id: string;
  status: Status;
  started: number;
  ended: number;
  error: string;
  summary?: Record<string, any> | null;
}

export interface JobInfo {
  id: string;
  last?: LastRun | null;
  last_success?: number;
  next?: number | null;
  queued?: boolean;
  recent: { id: string; status: Status; started: number; ended: number; added: number }[];
  running?: Op | null;
  schedule: string;
}

export interface RepoStats {
  at: number;
  total_size: number;
  total_uncompressed_size: number;
  compression_ratio: number;
  compression_space_saving: number;
  compression_progress: number;
  snapshots_count: number;
  total_blob_count: number;
}

export interface MaintenanceInfo {
  last?: { id: string; status: Status; ended: number; error: string } | null;
  next?: number | null;
  schedule: string;
}

export interface RepoInfo {
  id: string;
  url: string;
  running?: Op | null;
  stats?: RepoStats | null;
  prune: MaintenanceInfo;
  check: MaintenanceInfo;
}

export interface SystemInfo {
  array_started: boolean;
  data_dir_ok: boolean;
  installed: number;
  restic: string;
  scheduler_tick: number;
  secrets: { mode: Settings['secrets']; locked: boolean; problem: string };
  server: string;
  timezone: string;
}

export interface Overview {
  ok: boolean;
  now: number;
  config: Config;
  jobs: JobInfo[];
  repos: RepoInfo[];
  recent: Op[];
  running: Op[];
  system: SystemInfo;
}

export interface Snapshot {
  id: string;
  short_id: string;
  time: number;
  hostname: string;
  paths: string[];
  tags: string[];
  job: string;
  summary?: { added: number; size: number; files: number; new: number; changed: number; duration: number | null } | null;
}

export interface Entry {
  name: string;
  type: string;
  path: string;
  size: number | null;
  mtime: number | null;
  perm: string;
}
