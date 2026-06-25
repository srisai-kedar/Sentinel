const API_BASE = import.meta.env.VITE_API_BASE ?? '';
const ADMIN_KEY = import.meta.env.VITE_ADMIN_KEY ?? 'dev-admin-key';

export interface Snapshot {
  timestamp: string;
  requests_per_sec: number;
  allowed: number;
  blocked: number;
  top_offenders: Offender[];
  instance_id: string;
}

export interface Offender {
  client: string;
  route: string;
  blocked: number;
  total: number;
}

export interface LimitEntry {
  route: string;
  rule: {
    algorithm: string;
    capacity: number;
    refill_rate: number;
    window_sec: number;
  };
}

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      'X-Admin-Key': ADMIN_KEY,
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export function fetchStats(): Promise<Snapshot> {
  return api<Snapshot>('/api/v1/stats');
}

export function fetchLimits(): Promise<LimitEntry[]> {
  return api<LimitEntry[]>('/api/v1/limits');
}

export function updateLimit(route: string, body: LimitEntry['rule']): Promise<{ status: string }> {
  return api(`/api/v1/limits/${encodeURIComponent(route)}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  });
}

export function statsWebSocketURL(): string {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const host = import.meta.env.VITE_WS_HOST ?? window.location.host;
  return `${proto}://${host}/api/v1/ws/stats?key=${ADMIN_KEY}`;
}
