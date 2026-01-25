const API_BASE = window.location.origin;

export async function fetchHealth(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/health`);
    if (res.ok) {
      return true;
    }
  } catch {
    // ignore
  }
  return false;
}

export async function fetchLatency(): Promise<string> {
  const start = performance.now();
  try {
    const res = await fetch(`${API_BASE}/health`);
    if (res.ok) {
      return `${(performance.now() - start).toFixed(0)}ms`;
    }
  } catch {
    // ignore
  }
  return 'N/A';
}

export interface DNSStats {
  total_queries: number;
  success_rate: number;
  avg_response_time: string;
  total_domains: number;
  total_success: number;
  total_failed: number;
  top_domains: DomainInfo[];
}

export interface DomainInfo {
  domain: string;
  total_queries: number;
  query_types: Record<string, number>;
}

export interface CacheStats {
  size: number;
  hit_rate: number;
}

export interface StatsResponse {
  code: number;
  message: string;
  data: {
    dns: DNSStats;
    cache: CacheStats;
  };
}

export async function fetchStats(): Promise<StatsResponse | null> {
  try {
    const res = await fetch(`${API_BASE}/stats/dns`);
    const data = await res.json();
    if (data.code !== 0) {
      throw new Error(data.message);
    }
    return data;
  } catch (e) {
    console.error('Failed to fetch stats:', e);
    return null;
  }
}

export async function clearStats(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/stats/dns/clear`, { method: 'POST' });
    const data = await res.json();
    return data.code === 0;
  } catch {
    return false;
  }
}

export async function clearCache(): Promise<boolean> {
  try {
    const res = await fetch(`${API_BASE}/cache/dns/clear`, { method: 'POST' });
    const data = await res.json();
    return data.code === 0;
  } catch {
    return false;
  }
}

export function formatNumber(num: number | undefined | null): string {
  if (num == null || isNaN(num)) return '--';
  if (num >= 1000000) return `${(num / 1000000).toFixed(1)}M`;
  if (num >= 1000) return `${(num / 1000).toFixed(1)}K`;
  return num.toString();
}

export function formatPercent(value: number | undefined | null): string {
  if (value == null || isNaN(value)) return '--%';
  return `${value.toFixed(1)}%`;
}
