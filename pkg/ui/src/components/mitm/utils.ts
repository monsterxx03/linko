import { TrafficEvent } from '../../contexts/SSEContext';

const SENSITIVE_HEADERS = new Set([
  'authorization',
  'cookie',
  'set-cookie',
  'x-api-key',
  'x-access-token',
  'x-csrf-token',
  'proxy-authorization',
  'x-auth-token',
  'authentication',
  'token',
  'x-token',
  'api-key',
  'api-key-id',
  'api-secret',
  'x-secret',
  'x-password',
  'password',
  'session',
  'session-id',
  'x-session-id',
]);

export function maskSensitiveHeaderValue(key: string, value: string): string {
  const lowerKey = key.toLowerCase();
  if (SENSITIVE_HEADERS.has(lowerKey)) {
    // 保留前缀信息但隐藏实际内容
    if (lowerKey === 'authorization') {
      const parts = value.split(' ');
      if (parts.length > 1) {
        return `${parts[0]} ***`;
      }
    }
    return '***';
  }
  return value;
}

export function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString();
}

export function formatTimestamp(ts: number): string {
  return new Date(ts).toLocaleString();
}

export function toCurl(event: TrafficEvent): string {
  const { request, hostname } = event;
  if (!request) return '';

  let curl = `curl -X ${request.method || 'GET'}`;

  // Headers
  if (request.headers) {
    for (const [k, v] of Object.entries(request.headers)) {
      curl += ` \\\n  -H '${k}: ${v}'`;
    }
  }

  // Body
  if (request.body) {
    curl += ` \\\n  -d '${request.body.replace(/'/g, "'\\''")}'`;
  }

  // Build full URL with hostname
  // Default to https, but use http for IP or localhost
  const isLocalhost = hostname === 'localhost' || hostname?.startsWith('127.') || hostname?.startsWith('192.168.') || hostname?.startsWith('10.') || hostname?.startsWith('172.');
  const isIP = hostname ? /^\d+\.\d+\.\d+\.\d+$/.test(hostname) : false;
  const scheme = isLocalhost || isIP ? 'http' : 'https';
  const fullUrl = hostname && request.url ? `${scheme}://${hostname}${request.url}` : (request.url || '');

  curl += ` \\\n  '${fullUrl}'`;

  return curl;
}
