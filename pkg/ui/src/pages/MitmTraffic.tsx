import { useState } from 'react';
import ReactJson from 'react-json-view';
import { useTraffic } from '../hooks/useTraffic';
import { TrafficEvent } from '../contexts/SSEContext';

const METHOD_COLORS: Record<string, string> = {
  GET: 'bg-green-100 text-green-800',
  POST: 'bg-blue-100 text-blue-800',
  PUT: 'bg-yellow-100 text-yellow-800',
  DELETE: 'bg-red-100 text-red-800',
  PATCH: 'bg-purple-100 text-purple-800',
  HEAD: 'bg-gray-100 text-gray-800',
  OPTIONS: 'bg-indigo-100 text-indigo-800',
  CONNECT: 'bg-teal-100 text-teal-800',
};

const STATUS_COLORS: Record<number, string> = {
  2: 'bg-green-100 text-green-800',
  3: 'bg-blue-100 text-blue-800',
  4: 'bg-yellow-100 text-yellow-800',
  5: 'bg-red-100 text-red-800',
};

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString();
}

function formatMethod(m?: string): string {
  const c = METHOD_COLORS[m || ''] || 'bg-gray-100 text-gray-800';
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${c}">${m || ''}</span>`;
}

function formatStatus(s?: number): string {
  const c = s ? STATUS_COLORS[Math.floor(s / 100)] || 'bg-gray-100 text-gray-800' : 'bg-gray-100 text-gray-800';
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${c}">${s || ''}</span>`;
}

function formatHeaders(h?: Record<string, string>): string {
  return h ? Object.entries(h).map(([k, v]) => `${k}: ${v}`).join('\n') : '';
}

function toCurl(event: TrafficEvent): string {
  const { request } = event;
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

  curl += ` \\\n  '${request.url}'`;

  return curl;
}

function CopyButton({ text, label = 'Copy' }: { text: string; label?: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {}
  };

  return (
    <button
      onClick={handleCopy}
      className={`text-xs px-2.5 py-1 rounded border shadow-sm transition-all ${copied ? 'bg-green-50 border-green-300 text-green-700' : 'bg-white border-bg-300 text-bg-700 hover:bg-bg-50'}`}
      title="Copy to clipboard"
    >
      {copied ? '✓ Copied!' : label}
    </button>
  );
}

function JsonBody({ body, contentType }: { body: string; contentType?: string }) {
  if (!body) return null;

  const isJson = contentType?.includes('application/json');
  if (isJson) {
    try {
      const parsed = JSON.parse(body);
      return (
        <div className="relative pr-20">
          <CopyButton text={body} />
          <ReactJson
            src={parsed}
            theme="rjv-default"
            collapsed={2}
            displayDataTypes={false}
            enableClipboard={false}
            iconStyle="triangle"
            style={{ backgroundColor: 'transparent', padding: '8px', borderRadius: '4px' }}
          />
        </div>
      );
    } catch {}
  }
  return (
    <div className="relative pr-20">
      <CopyButton text={body} />
      <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{body}</pre>
    </div>
  );
}

function CollapsibleSection({ title, defaultExpanded, children }: {
  title: string;
  defaultExpanded?: boolean;
  children: React.ReactNode;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  return (
    <div className="p-3 bg-bg-50 rounded-lg">
      <button onClick={() => setExpanded(!expanded)} className="flex items-center justify-between w-full text-left hover:bg-bg-100 rounded px-2 py-1 -mx-2 -mt-1 transition-colors">
        <span className="text-xs font-medium text-bg-600">{title}</span>
        <svg className={`w-4 h-4 text-bg-400 transition-transform ${expanded ? 'rotate-180' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      <div className={`mt-2 ${expanded ? '' : 'hidden'}`}>{children}</div>
    </div>
  );
}

function TrafficItem({ event, bodyExpanded }: { event: TrafficEvent; bodyExpanded: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const { request, response } = event;
  const hasReq = request !== undefined;
  const hasResp = response !== undefined;
  const complete = event.direction === 'complete' || (hasReq && hasResp);

  const leftInfo = complete && hasReq && hasResp ? (
    <>
      <span dangerouslySetInnerHTML={{ __html: formatMethod(request?.method) }} />
      <span className="text-sm font-medium text-bg-800 truncate max-w-xs">{request?.url || event.hostname}</span>
      <span dangerouslySetInnerHTML={{ __html: formatStatus(response?.status_code) }} />
    </>
  ) : hasReq ? (
    <>
      <span dangerouslySetInnerHTML={{ __html: formatMethod(request?.method) }} />
      <span className="text-sm font-medium text-bg-800 truncate max-w-xs">{request?.url}</span>
    </>
  ) : hasResp ? (
    <>
      <span dangerouslySetInnerHTML={{ __html: formatStatus(response?.status_code) }} />
      <span className="text-sm font-medium text-bg-800">{response?.status}</span>
    </>
  ) : (
    <span className="text-sm font-medium text-bg-800">{event.direction}</span>
  );

  return (
    <div id={`traffic-${event.id}`} className="bg-white rounded-xl border border-bg-200 p-4 mb-3 animate-fade-in">
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-3">{leftInfo}</div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-bg-400 font-mono" title={event.request_id || event.id}>{event.request_id ? event.request_id.slice(-8) : event.id.slice(0, 8)}</span>
          <span className="text-xs text-bg-400">{formatTime(event.timestamp)}</span>
          <span className="text-xs text-bg-400">{event.hostname}</span>
          {response?.latency !== undefined && <span className="text-xs text-bg-400">{response.latency}ms</span>}
          <button onClick={() => setExpanded(!expanded)} className="text-xs text-bg-400 hover:text-bg-600 focus:outline-none">
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={expanded ? 'M5 15l7-7 7 7' : 'M19 9l-7 7-7-7'} />
            </svg>
          </button>
        </div>
      </div>

      {expanded && (
        <div className="mt-3 space-y-3">
          {hasReq && (
            <div className="flex justify-end">
              <CopyButton text={toCurl(event)} label="Copy as cURL" />
            </div>
          )}
          {hasReq && (
            <>
              <CollapsibleSection title="Request Headers">
                <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{formatHeaders(request?.headers)}</pre>
              </CollapsibleSection>
              {request?.body && (
                <CollapsibleSection title="Request Body" defaultExpanded={bodyExpanded}>
                  <JsonBody body={request.body} contentType={request.content_type} />
                </CollapsibleSection>
              )}
            </>
          )}
          {hasResp && (
            <>
              <CollapsibleSection title="Response Headers">
                <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{formatHeaders(response?.headers)}</pre>
              </CollapsibleSection>
              {response?.body && (
                <CollapsibleSection title="Response Body" defaultExpanded={bodyExpanded}>
                  <JsonBody body={response.body} contentType={response.content_type} />
                </CollapsibleSection>
              )}
            </>
          )}
        </div>
      )}
    </div>
  );
}

function MitmTraffic() {
  const { events, isConnected, error, search, setSearch, setAutoScroll, clear, reconnect } = useTraffic({ maxEvents: 100, autoScroll: true });
  const [collapseVer, setCollapseVer] = useState(0);
  const [bodyExpanded, setBodyExpanded] = useState(true);

  return (
    <div className="tab-section">
      <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <div className={`w-2.5 h-2.5 rounded-full ${isConnected ? 'bg-emerald-500' : 'bg-red-500 animate-pulse'}`} />
              <span className="text-sm font-medium text-bg-800">MITM Proxy</span>
              <span className={`text-sm ${isConnected ? 'text-emerald-600' : 'text-red-500'}`}>{isConnected ? 'Online' : 'Connecting...'}</span>
            </div>
            <div className="h-5 w-px bg-bg-200" />
            <span className="text-sm text-bg-600">Connected clients: <span className="font-medium text-bg-800">{events.length > 0 ? '1' : '0'}</span></span>
          </div>
          <div className="flex gap-3">
            <button onClick={() => { setBodyExpanded(false); setCollapseVer(v => v + 1); }} className="px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200">Collapse Bodies</button>
            <button onClick={clear} className="px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200">Clear Traffic</button>
            {error && <button onClick={reconnect} className="px-4 py-2 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100">Reconnect</button>}
          </div>
        </div>
      </div>

      <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <label className="text-sm font-medium text-bg-600">Search:</label>
            <input type="text" value={search} onChange={e => setSearch(e.target.value)} placeholder="Search URLs, domains..." className="px-3 py-1.5 border border-bg-300 rounded-lg text-sm focus:ring-2 focus:ring-accent-500 w-64" />
          </div>
          <div className="flex items-center gap-2 ml-auto">
            <input type="checkbox" id="auto-scroll" checked onChange={e => setAutoScroll(e.target.checked)} className="rounded text-accent-500 focus:ring-accent-500" />
            <label htmlFor="auto-scroll" className="text-sm text-bg-600">Auto scroll</label>
          </div>
        </div>
      </div>

      <div className="bg-white rounded-xl border border-bg-200">
        <div className="px-5 py-4 border-b border-bg-100 flex items-center justify-between">
          <h2 className="font-semibold text-bg-800">MITM Traffic</h2>
          <span className="text-xs text-bg-400">Live</span>
        </div>
        <div className="max-h-[600px] overflow-y-auto">
          <div className="p-4">
            {!isConnected && !error && <div className="text-center py-8 text-bg-400">Connecting to SSE endpoint...</div>}
            {error && <div className="text-center py-8 text-red-400">Error: {error}</div>}
            {isConnected && events.length === 0 && <div className="text-center py-8 text-bg-400">No traffic data available</div>}
            {events.map(e => <TrafficItem key={`${e.id}-${collapseVer}`} event={e} bodyExpanded={bodyExpanded} />)}
          </div>
        </div>
      </div>
    </div>
  );
}

export default MitmTraffic;
