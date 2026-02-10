import React, { useState } from 'react';

interface MessageBubbleProps {
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string | string[];
  isStreaming?: boolean;
  tokens?: number;
  tool_calls?: Array<{
    id: string;
    type: string;
    function: {
      name: string;
      arguments: string;
    };
  }>;
  timestamp?: number;
}

function formatTime(ts?: number): string {
  if (!ts) return '';
  return new Date(ts).toLocaleTimeString();
}

// Decode HTML entities and Unicode escapes for tag detection
function decodeForTagCheck(content: string): string {
  return content
    .replace(/&lt;/gi, '<')
    .replace(/&gt;/gi, '>')
    .replace(/&quot;/gi, '"')
    .replace(/&#39;/gi, "'")
    .replace(/&amp;/gi, '&') // must be last
    .replace(/\\u003c/g, '<')
    .replace(/\\u003e/g, '>');
}

// Check if content is wrapped by matching XML/HTML tags at start and end
function hasXmlTags(content: string): boolean {
  const decoded = decodeForTagCheck(content);
  // Match opening tag at start: <tagName...> or <tagName ...>
  const openMatch = decoded.match(/^<([a-zA-Z][a-zA-Z0-9-]*)[^>]*>/);
  if (!openMatch) return false;

  const tagName = openMatch[1];
  // Match closing tag at end: </tagName>
  const closeRegex = new RegExp(`</${tagName}>$`);
  return closeRegex.test(decoded);
}

// Extract tag name from content (only call when hasXmlTags is true)
function extractFirstTagName(content: string): string {
  const decoded = decodeForTagCheck(content);
  const match = decoded.match(/^<([a-zA-Z][a-zA-Z0-9-]*)/);
  return match ? match[1] : 'unknown';
}

// Collapsible content block for XML/HTML content
function CollapsibleContent({ content, index }: { content: string; index: number }) {
  const [collapsed, setCollapsed] = useState(true);

  const hasTags = hasXmlTags(content);
  const tagName = extractFirstTagName(content);

  if (!hasTags) {
    return <>{formatContent(content)}</>;
  }

  return (
    <div className="border border-bg-200 rounded-lg overflow-hidden my-2">
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-bg-50 transition-colors bg-bg-50"
      >
        <svg
          className={`w-4 h-4 text-bg-500 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-xs text-bg-500 font-mono">
          [{index}] &lt;{tagName}&gt;
        </span>
        <span className="text-xs text-bg-400 ml-auto">
          {collapsed ? 'Expand' : 'Collapse'}
        </span>
      </button>
      {!collapsed && (
        <div className="px-3 py-2 bg-bg-50 border-t border-bg-200">
          <pre className="text-xs font-mono text-bg-700 whitespace-pre-wrap break-all">
            {content}
          </pre>
        </div>
      )}
    </div>
  );
}

function formatContent(content: string): React.ReactNode {
  // Basic markdown-like formatting
  const lines = content.split('\n');
  const elements: React.ReactNode[] = [];

  lines.forEach((line, i) => {
    if (i > 0) {
      elements.push(<br key={`br-${i}`} />);
    }

    // Code blocks
    if (line.startsWith('```')) {
      return;
    }

    // Inline code
    if (line.includes('`')) {
      const parts = line.split(/`([^`]+)`/g);
      parts.forEach((part, j) => {
        if (j % 2 === 1) {
          elements.push(<code key={`code-${i}-${j}`} className="bg-bg-100 px-1 py-0.5 rounded text-sm font-mono text-pink-600">{part}</code>);
        } else if (part) {
          elements.push(<span key={`text-${i}-${j}`}>{part}</span>);
        }
      });
    } else if (line) {
      elements.push(<span key={`text-${i}`}>{line}</span>);
    }
  });

  return elements;
}

function ToolCallPanel({ calls }: { calls: Array<{ id: string; function: { name: string; arguments: string } }> }) {
  const [expanded, setExpanded] = useState(false);

  if (!calls || calls.length === 0) return null;

  return (
    <div className="my-2">
      {calls.map((call) => (
        <div key={call.id} className="bg-purple-50 border border-purple-200 rounded-lg overflow-hidden">
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-purple-100 transition-colors"
          >
            <svg className="w-4 h-4 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
            <span className="text-sm font-medium text-purple-800">{call.function.name}</span>
            <span className="text-xs text-purple-500 ml-auto">
              {expanded ? 'Hide' : 'Show'} arguments
            </span>
          </button>
          {expanded && (
            <div className="px-3 py-2 bg-purple-100 border-t border-purple-200">
              <pre className="text-xs font-mono text-purple-900 whitespace-pre-wrap break-all">
                {call.function.arguments}
              </pre>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

export function MessageBubble({ role, content, isStreaming, tokens, tool_calls, timestamp }: MessageBubbleProps) {
  const isUser = role === 'user';

  const roleColors = {
    user: 'bg-blue-100 border-blue-200',
    assistant: 'bg-white border-bg-200',
    system: 'bg-yellow-50 border-yellow-200',
    tool: 'bg-purple-50 border-purple-200',
  };

  const roleIcon = {
    user: (
      <svg className="w-4 h-4 text-blue-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
      </svg>
    ),
    assistant: (
      <svg className="w-4 h-4 text-emerald-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
      </svg>
    ),
    system: (
      <svg className="w-4 h-4 text-yellow-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
    ),
    tool: (
      <svg className="w-4 h-4 text-purple-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
      </svg>
    ),
  };

  const roleLabel = {
    user: 'You',
    assistant: 'Assistant',
    system: 'System',
    tool: 'Tool',
  };

  return (
    <div className="flex gap-3">
      {/* Avatar */}
      <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center ${roleColors[role].replace('border', 'bg')}`}>
        {roleIcon[role]}
      </div>

      {/* Message bubble */}
      <div className="flex-1 max-w-[80%]">
        <div className="flex items-center gap-2 mb-1">
          <span className={`text-xs font-medium ${isUser ? 'text-blue-700' : 'text-bg-600'}`}>
            {roleLabel[role]}
          </span>
          {timestamp && (
            <span className="text-xs text-bg-400">{formatTime(timestamp)}</span>
          )}
          {isStreaming && (
            <span className="flex items-center gap-1 text-xs text-emerald-600">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              Streaming
            </span>
          )}
        </div>

        <div className={`rounded-xl p-4 border ${roleColors[role]} rounded-tl-sm`}>
          {/* Tool calls */}
          {tool_calls && tool_calls.length > 0 && (
            <ToolCallPanel calls={tool_calls} />
          )}

          {/* Content */}
          <div className={`text-sm leading-relaxed ${isUser ? 'text-blue-900' : 'text-bg-800'}`}>
            {Array.isArray(content) ? (
              <div className="space-y-2">
                {content.map((c, i) => (
                  <div key={i} className="pb-2 border-b border-bg-200 last:border-0 last:pb-0">
                    <CollapsibleContent content={c} index={i} />
                  </div>
                ))}
              </div>
            ) : (
              <CollapsibleContent content={content} index={0} />
            )}
            {isStreaming && (
              <span className="inline-block w-2 h-4 ml-1 bg-emerald-500 animate-pulse" />
            )}
          </div>

          {/* Token count */}
          {tokens && tokens > 0 && (
            <div className="mt-2 pt-2 border-t border-bg-200/50 flex items-center gap-2 text-xs text-bg-400">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
              </svg>
              <span>{tokens} tokens</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
