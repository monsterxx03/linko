import { useState, useMemo, useCallback } from 'react';
import ReactJson from 'react-json-view';
import { TrafficEvent } from '../../contexts/SSEContext';
import { Badge, getMethodColor, getStatusColor } from './shared/Badge';
import { CopyButton } from './shared/CopyButton';
import { CollapsibleSection } from './shared/CollapsibleSection';
import { maskSensitiveHeaderValue, formatTime, toCurl } from './utils';

// Internal HeadersDisplay component
interface HeadersDisplayProps {
  headers?: Record<string, string>;
}

function HeadersDisplay({ headers }: HeadersDisplayProps) {
  const [showRaw, setShowRaw] = useState(false);

  const toggleShowRaw = useCallback(() => {
    setShowRaw(prev => !prev);
  }, []);

  const formattedHeaders = useMemo(() => {
    if (!headers || Object.keys(headers).length === 0) {
      return null;
    }

    return Object.entries(headers).map(([key, value]) => {
      const displayValue = showRaw ? value : maskSensitiveHeaderValue(key, value);
      return `${key}: ${displayValue}`;
    }).join('\n');
  }, [headers, showRaw]);

  if (!formattedHeaders) {
    return <div className="text-xs text-bg-400 italic py-1">No headers</div>;
  }

  return (
    <div className="relative pr-20">
      <button
        onClick={toggleShowRaw}
        className="absolute top-0 right-0 text-xs px-2.5 py-1 rounded-md border shadow-sm transition-all duration-150 bg-white border-bg-300 text-bg-700 hover:bg-bg-50 hover:border-bg-400"
        title={showRaw ? "Hide sensitive values" : "Show raw values"}
      >
        {showRaw ? "Hide Raw" : "Show Raw"}
      </button>
      <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{formattedHeaders}</pre>
    </div>
  );
}

// Internal JsonBody component
interface JsonBodyProps {
  body: string;
  contentType?: string;
}

function JsonBody({ body, contentType }: JsonBodyProps) {
  if (!body) return null;

  const isJson = contentType?.includes('application/json');

  if (isJson) {
    try {
      const parsed = JSON.parse(body);
      return (
        <div className="relative pr-20">
          <CopyButton text={body} className="absolute top-0 right-0 z-10" />
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
    } catch {
      // Fall through to plain text display
    }
  }

  return (
    <div className="relative pr-20">
      <CopyButton text={body} className="absolute top-0 right-0 z-10" />
      <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">{body}</pre>
    </div>
  );
}

export interface TrafficItemProps {
  event: TrafficEvent;
  bodyExpanded: boolean;
}

export function TrafficItem({ event, bodyExpanded }: TrafficItemProps) {
  const [expanded, setExpanded] = useState(false);
  const { request, response } = event;
  const hasReq = request !== undefined;
  const hasResp = response !== undefined;
  const complete = event.direction === 'complete' || (hasReq && hasResp);

  const toggleExpanded = useCallback(() => {
    setExpanded(prev => !prev);
  }, []);

  const leftInfo = useMemo(() => {
    if (complete && hasReq && hasResp) {
      return (
        <>
          <Badge colorClass={getMethodColor(request?.method)}>{request?.method || ''}</Badge>
          <span className="text-sm font-medium text-bg-800 truncate max-w-xs">{request?.url || event.hostname}</span>
          <Badge colorClass={getStatusColor(response?.status_code)}>{response?.status_code || ''}</Badge>
        </>
      );
    }

    if (hasReq) {
      return (
        <>
          <Badge colorClass={getMethodColor(request?.method)}>{request?.method || ''}</Badge>
          <span className="text-sm font-medium text-bg-800 truncate max-w-xs">{request?.url}</span>
        </>
      );
    }

    if (hasResp) {
      return (
        <>
          <Badge colorClass={getStatusColor(response?.status_code)}>{response?.status_code || ''}</Badge>
          <span className="text-sm font-medium text-bg-800">{response?.status}</span>
        </>
      );
    }

    return <span className="text-sm font-medium text-bg-800">{event.direction}</span>;
  }, [complete, hasReq, hasResp, request, response, event]);

  const curlCommand = useMemo(() => toCurl(event), [event]);

  return (
    <div
      id={`traffic-${event.id}`}
      className="bg-white rounded-xl border border-bg-200 p-4 mb-3 shadow-sm hover:shadow-md transition-shadow duration-200"
    >
      <div className="flex items-center justify-between mb-2">
        <div className="flex items-center gap-3 min-w-0">{leftInfo}</div>
        <div className="flex items-center gap-3 flex-shrink-0">
          <span className="text-xs text-bg-400 font-mono" title={event.request_id || event.id}>
            {event.request_id ? event.request_id.slice(-8) : event.id.slice(0, 8)}
          </span>
          <span className="text-xs text-bg-400">{formatTime(event.timestamp)}</span>
          <span className="text-xs text-bg-400 truncate max-w-[100px]">{event.hostname}</span>
          {response?.latency !== undefined && (
            <span className="text-xs text-bg-400 font-mono bg-bg-100 px-1.5 py-0.5 rounded">
              {response.latency}ms
            </span>
          )}
          <button
            onClick={toggleExpanded}
            className="text-xs text-bg-400 hover:text-bg-600 focus:outline-none p-1 rounded hover:bg-bg-50 transition-colors duration-150"
            title={expanded ? "Collapse details" : "Expand details"}
          >
            <svg className="w-4 h-4 transition-transform duration-200" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d={expanded ? 'M5 15l7-7 7 7' : 'M19 9l-7 7-7-7'} />
            </svg>
          </button>
        </div>
      </div>

      {expanded && (
        <div className="mt-4 space-y-3 border-t border-bg-100 pt-4">
          {hasReq && (
            <div className="flex justify-end">
              <CopyButton text={curlCommand} label="Copy as cURL" />
            </div>
          )}

          {hasReq && (
            <>
              <CollapsibleSection title="Request Headers">
                <HeadersDisplay headers={request?.headers} />
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
                <HeadersDisplay headers={response?.headers} />
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
