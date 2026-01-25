import React, { useState } from 'react';
import ReactJson from 'react-json-view';
import { useTraffic, TrafficEvent } from '../hooks/useTraffic';

function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString();
}

function formatMethod(method?: string): string {
  const methodColors: Record<string, string> = {
    GET: 'bg-green-100 text-green-800',
    POST: 'bg-blue-100 text-blue-800',
    PUT: 'bg-yellow-100 text-yellow-800',
    DELETE: 'bg-red-100 text-red-800',
    PATCH: 'bg-purple-100 text-purple-800',
    HEAD: 'bg-gray-100 text-gray-800',
    OPTIONS: 'bg-indigo-100 text-indigo-800',
    CONNECT: 'bg-teal-100 text-teal-800',
  };
  const color = methodColors[method || ''] || 'bg-gray-100 text-gray-800';
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${color}">${method || ''}</span>`;
}

function formatStatusCode(statusCode?: number): string {
  let color = 'bg-gray-100 text-gray-800';
  if (statusCode && statusCode >= 200 && statusCode < 300) {
    color = 'bg-green-100 text-green-800';
  } else if (statusCode && statusCode >= 300 && statusCode < 400) {
    color = 'bg-blue-100 text-blue-800';
  } else if (statusCode && statusCode >= 400 && statusCode < 500) {
    color = 'bg-yellow-100 text-yellow-800';
  } else if (statusCode && statusCode >= 500) {
    color = 'bg-red-100 text-red-800';
  }
  return `<span class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${color}">${statusCode || ''}</span>`;
}

function formatHeaders(headers?: Record<string, string>): string {
  if (!headers) return '';
  return Object.entries(headers)
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n');
}

interface JsonBodyProps {
  body: string;
  contentType?: string;
}

function JsonBody({ body, contentType }: JsonBodyProps) {
  if (!body) return null;

  const isJson = contentType && contentType.includes('application/json');

  if (isJson) {
    try {
      const parsed = JSON.parse(body);
      return (
        <ReactJson
          src={parsed}
          theme="rjv-default"
          collapsed={2}
          displayDataTypes={false}
          enableClipboard={false}
          iconStyle="triangle"
          style={{ backgroundColor: 'transparent', padding: '8px', borderRadius: '4px' }}
        />
      );
    } catch {
      return <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{body}</pre>;
    }
  }

  return <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{body}</pre>;
}

interface SectionProps {
  title: string;
  sectionId: string;
  children: React.ReactNode;
  defaultExpanded?: boolean;
}

function CollapsibleSection({ title, sectionId, children, defaultExpanded = false }: SectionProps) {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  return (
    <div className="p-3 bg-bg-50 rounded-lg">
      <button
        onClick={() => setIsExpanded(!isExpanded)}
        className="flex items-center justify-between w-full text-left hover:bg-bg-100 rounded px-2 py-1 -mx-2 -mt-1 transition-colors"
      >
        <h4 className="text-xs font-medium text-bg-600">{title}</h4>
        <svg
          id={`${sectionId}-icon`}
          className={`w-4 h-4 text-bg-400 transition-transform ${isExpanded ? 'rotate-180' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      <div id={sectionId} className={`mt-2 ${isExpanded ? '' : 'hidden'}`}>
        {children}
      </div>
    </div>
  );
}

interface TrafficItemProps {
  event: TrafficEvent;
  isBodyExpanded: boolean;
}

function TrafficItem({ event, isBodyExpanded }: TrafficItemProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  // 只有在全局折叠时，body 才折叠；否则保持用户选择的展开状态
  const bodyExpanded = isBodyExpanded;
  const hasRequest = event.request !== undefined;
  const hasResponse = event.response !== undefined;
  const isComplete = event.direction === 'complete' || (hasRequest && hasResponse);

  const leftSection = () => {
    if (isComplete && hasRequest && hasResponse) {
      return (
        <div className="flex items-center gap-2">
          <span dangerouslySetInnerHTML={{ __html: formatMethod(event.request?.method) }} />
          <span className="text-sm font-medium text-bg-800 truncate max-w-xs">
            {event.request?.url || event.hostname}
          </span>
          <span dangerouslySetInnerHTML={{ __html: formatStatusCode(event.response?.status_code) }} />
        </div>
      );
    } else if (hasRequest) {
      return (
        <div className="flex items-center gap-2">
          <span dangerouslySetInnerHTML={{ __html: formatMethod(event.request?.method) }} />
          <span className="text-sm font-medium text-bg-800 truncate max-w-xs">{event.request?.url}</span>
        </div>
      );
    } else if (hasResponse) {
      return (
        <div className="flex items-center gap-2">
          <span dangerouslySetInnerHTML={{ __html: formatStatusCode(event.response?.status_code) }} />
          <span className="text-sm font-medium text-bg-800">{event.response?.status}</span>
        </div>
      );
    }
    return <span className="text-sm font-medium text-bg-800">{event.direction}</span>;
  };

  const rightSection = () => (
    <div className="flex items-center gap-2">
      <span className="text-xs text-bg-400 font-mono">{event.id.slice(0, 8)}</span>
      <span className="text-xs text-bg-400">{formatTime(event.timestamp)}</span>
      <span className="text-xs text-bg-400">{event.hostname}</span>
      {event.response?.latency !== undefined && (
        <span className="text-xs text-bg-400">{event.response.latency}ms</span>
      )}
    </div>
  );

  return (
    <div
      id={`traffic-${event.id}`}
      className="traffic-item bg-white rounded-xl border border-bg-200 p-4 mb-3 animate-fade-in"
    >
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-3">{leftSection()}</div>
        <div className="flex items-center gap-2">
          {rightSection()}
          <button
            onClick={() => setIsExpanded(!isExpanded)}
            className="text-xs text-bg-400 hover:text-bg-600 focus:outline-none"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d={isExpanded ? 'M5 15l7-7 7 7' : 'M19 9l-7 7-7-7'}
              />
            </svg>
          </button>
        </div>
      </div>

      {isExpanded && (
        <div className="traffic-details mt-3 space-y-3">
          {hasRequest && (
            <>
              <CollapsibleSection title="Request Headers" sectionId={`traffic-${event.id}-req-headers`}>
                <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">
                  {formatHeaders(event.request?.headers)}
                </pre>
              </CollapsibleSection>
              {event.request?.body && (
                <CollapsibleSection title="Request Body" sectionId={`traffic-${event.id}-req-body`} defaultExpanded={bodyExpanded}>
                  <JsonBody body={event.request?.body} contentType={event.request?.content_type} />
                </CollapsibleSection>
              )}
            </>
          )}
          {hasResponse && (
            <>
              <CollapsibleSection title="Response Headers" sectionId={`traffic-${event.id}-resp-headers`}>
                <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">
                  {formatHeaders(event.response?.headers)}
                </pre>
              </CollapsibleSection>
              {event.response?.body && (
                <CollapsibleSection title="Response Body" sectionId={`traffic-${event.id}-resp-body`} defaultExpanded={bodyExpanded}>
                  <JsonBody body={event.response?.body} contentType={event.response?.content_type} />
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
  const {
    events,
    isConnected,
    error,
    filter,
    search,
    setFilter,
    setSearch,
    setAutoScroll,
    clear,
    reconnect,
  } = useTraffic({ maxEvents: 100, autoScroll: true });
  const [collapseVersion, setCollapseVersion] = useState(0);
  const [isBodyExpanded, setIsBodyExpanded] = useState(true);

  const collapseBodies = () => {
    setIsBodyExpanded(false);
    setCollapseVersion(v => v + 1);
  };

  return (
    <div className="tab-section">
      {/* MITM Status Bar */}
      <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <div
                className={`w-2.5 h-2.5 rounded-full ${isConnected ? 'bg-emerald-500' : 'bg-red-500 animate-pulse'}`}
              />
              <span className="text-sm font-medium text-bg-800">MITM Proxy</span>
              <span className={`text-sm ${isConnected ? 'text-emerald-600' : 'text-red-500'}`}>
                {isConnected ? 'Online' : 'Connecting...'}
              </span>
            </div>
            <div className="h-5 w-px bg-bg-200" />
            <div className="text-sm text-bg-600">
              Connected clients: <span className="font-medium text-bg-800">{events.length > 0 ? '1' : '0'}</span>
            </div>
          </div>
          <div className="flex gap-3">
            <button
              onClick={collapseBodies}
              className="btn-action px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200"
            >
              Collapse Bodies
            </button>
            <button
              onClick={clear}
              className="btn-action px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200"
            >
              Clear Traffic
            </button>
            {error && (
              <button
                onClick={reconnect}
                className="btn-action px-4 py-2 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100"
              >
                Reconnect
              </button>
            )}
          </div>
        </div>
      </div>

      {/* MITM Traffic Filters */}
      <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6">
        <div className="flex flex-wrap items-center gap-3">
          <div className="flex items-center gap-2">
            <label className="text-sm font-medium text-bg-600">Filter:</label>
            <select
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              className="px-3 py-1.5 border border-bg-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-accent-500 focus:border-transparent"
            >
              <option value="all">All Traffic</option>
              <option value="requests">Requests Only</option>
              <option value="responses">Responses Only</option>
            </select>
          </div>
          <div className="flex items-center gap-2">
            <label className="text-sm font-medium text-bg-600">Search:</label>
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search URLs, domains..."
              className="px-3 py-1.5 border border-bg-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-accent-500 focus:border-transparent w-64"
            />
          </div>
          <div className="flex items-center gap-2 ml-auto">
            <input
              type="checkbox"
              id="auto-scroll"
              checked
              onChange={(e) => setAutoScroll(e.target.checked)}
              className="rounded text-accent-500 focus:ring-accent-500"
            />
            <label htmlFor="auto-scroll" className="text-sm text-bg-600">
              Auto scroll
            </label>
          </div>
        </div>
      </div>

      {/* MITM Traffic List */}
      <div className="bg-white rounded-xl border border-bg-200">
        <div className="px-5 py-4 border-b border-bg-100 flex items-center justify-between">
          <h2 className="font-semibold text-bg-800">MITM Traffic</h2>
          <span className="text-xs text-bg-400">Live</span>
        </div>
        <div className="max-h-[600px] overflow-y-auto">
          <div id="mitm-traffic-list" className="p-4">
            {!isConnected && !error && (
              <div className="text-center py-8 text-bg-400">Connecting to SSE endpoint...</div>
            )}
            {error && <div className="text-center py-8 text-red-400">Error: {error}</div>}
            {isConnected && events.length === 0 && (
              <div className="text-center py-8 text-bg-400">No traffic data available</div>
            )}
            {events.map((event) => (
              <TrafficItem key={`${event.id}-${collapseVersion}`} event={event} isBodyExpanded={isBodyExpanded} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

export default MitmTraffic;
