import React, { useState, useEffect } from 'react';
import ReactMarkdown from 'react-markdown';

export interface ToolDef {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

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
  streaming_tool_calls?: Array<{
    id?: string;
    name?: string;
    arguments: string;
  }>;
  timestamp?: number;
  system_prompts?: string[];
  tools?: ToolDef[];
  thinking?: string;
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

// Check if content is wrapped by <system-reminder> tags
function hasSystemReminderTag(content: string): boolean {
  const decoded = decodeForTagCheck(content);
  // Match opening <system-reminder> tag at start
  const openMatch = /^<system-reminder[\s>]/i.test(decoded);
  if (!openMatch) return false;

  // Match closing </system-reminder> tag at end
  const closeRegex = /<\/system-reminder>$/i;
  return closeRegex.test(decoded);
}

// CopyButton shows a copy button that copies text to clipboard
function CopyButton({ text, className = '' }: { text: string; className?: string }) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <button
      onClick={handleCopy}
      className={`p-1 rounded hover:bg-slate-100 transition-colors ${className}`}
      title="Copy"
    >
      {copied ? (
        <svg className="w-3.5 h-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
        </svg>
      ) : (
        <svg className="w-3.5 h-3.5 text-slate-400 hover:text-slate-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z" />
        </svg>
      )}
    </button>
  );
}

// System prompt content - multiple prompts are shown with separators
function SystemContent({ content }: { content: string[] }) {
  if (content.length === 0) {
    return null;
  }

  if (content.length === 1) {
    return <SingleSystemPrompt content={content[0]} />;
  }

  // Multiple system prompts with separators
  return (
    <div className="space-y-3">
      {content.map((c, i) => (
        <div key={i} className="relative pl-6">
          {/* Separator line */}
          {i > 0 && (
            <div className="absolute left-2 top-[-12px] w-0.5 h-3 bg-amber-200" />
          )}
          {/* Index badge */}
          <span className="absolute left-0 top-1 w-5 h-5 rounded-full bg-amber-200 text-amber-700 text-xs flex items-center justify-center font-medium">
            {i + 1}
          </span>
          <div className="border border-amber-200 rounded-lg overflow-hidden bg-amber-50/50">
            <SingleSystemPrompt content={c} />
          </div>
        </div>
      ))}
    </div>
  );
}

function SingleSystemPrompt({ content }: { content: string }) {
  const [collapsed, setCollapsed] = useState(true);
  const previewLength = 100;
  const shouldCollapse = content.length > previewLength;

  if (!shouldCollapse) {
    return (
      <div className="flex items-start gap-1 px-2 py-1 group">
        <div className="text-xs text-slate-600 flex-1"><MarkdownContent content={content} /></div>
        <CopyButton text={content} className="opacity-0 group-hover:opacity-100" />
      </div>
    );
  }

  const preview = content.substring(0, previewLength) + '...';

  return (
    <>
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-amber-100 transition-colors"
      >
        <svg
          className={`w-3 h-3 text-amber-600 transition-transform ${collapsed ? '' : 'rotate-90'}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-xs text-amber-700 flex-1">
          {collapsed ? preview : ''}
        </span>
        <span className="text-xs text-amber-500 whitespace-nowrap">
          {collapsed ? '展开' : '折叠'}
        </span>
      </button>
      {!collapsed && (
        <div className="px-3 py-2 bg-amber-50 border-t border-amber-200">
          <div className="flex items-start gap-1 group">
            <div className="text-xs text-slate-700 flex-1"><MarkdownContent content={content} /></div>
            <CopyButton text={content} />
          </div>
        </div>
      )}
    </>
  );
}

// Collapsible content block for system-reminder tags or plain text content
function CollapsibleContent({ content, index }: { content: string; index: number }) {
  const hasTags = hasSystemReminderTag(content);

  if (!hasTags) {
    return <CollapsibleMarkdown content={content} index={index} />;
  }

  const tagName = 'system-reminder';
  const [collapsed, setCollapsed] = useState(false);

  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden my-2">
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-slate-50 transition-colors bg-slate-50"
      >
        <svg
          className={`w-4 h-4 text-slate-500 transition-transform ${collapsed ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-xs text-slate-500 font-mono">
          [{index}] &lt;{tagName}&gt;
        </span>
        <span className="text-xs text-slate-400 ml-auto">
          {collapsed ? 'Hide' : 'Show'}
        </span>
      </button>
      {collapsed && (
        <div className="px-3 py-2 bg-slate-50 border-t border-slate-200">
          <pre className="text-sm font-mono text-slate-700 whitespace-pre-wrap break-all">
            {content}
          </pre>
        </div>
      )}
    </div>
  );
}

// CollapsibleMarkdown renders long content with collapse functionality (plain text, no markdown)
const COLLAPSE_THRESHOLD = 500;

function CollapsibleMarkdown({ content, index }: { content: string; index: number }) {
  const [collapsed, setCollapsed] = useState(content.length > COLLAPSE_THRESHOLD);

  if (!collapsed) {
    return (
      <div className="text-sm text-slate-800 whitespace-pre-wrap break-all">
        {content}
      </div>
    );
  }

  const preview = content.substring(0, COLLAPSE_THRESHOLD) + '...';

  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden my-2">
      <button
        onClick={() => setCollapsed(!collapsed)}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-slate-50 transition-colors bg-slate-50"
      >
        <svg
          className={`w-4 h-4 text-slate-500 transition-transform ${collapsed ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="text-xs text-slate-500 font-mono flex-1 text-left">
          [{index}] {preview}
        </span>
        <span className="text-xs text-slate-400 whitespace-nowrap">
          {collapsed ? '展开' : '收起'}
        </span>
      </button>
      {!collapsed && (
        <div className="px-3 py-2 bg-slate-50 border-t border-slate-200">
          <div className="text-sm text-slate-800 whitespace-pre-wrap break-all">
            {content}
          </div>
        </div>
      )}
    </div>
  );
}

// MarkdownContent renders markdown content with proper styling (used for system prompts and tool descriptions)
function MarkdownContent({ content, className = '' }: { content: string; className?: string }) {
  return (
    <div className={`markdown-content ${className}`}>
      <ReactMarkdown
        components={{
          code({ children, className, node, ...props }) {
            const isInline = !className;
            return (
              <code
                className={`${className || ''} ${isInline ? 'bg-slate-100 px-1.5 py-0.5 rounded text-sm font-mono text-pink-600' : 'block bg-slate-50 p-3 rounded-lg overflow-x-auto text-xs font-mono my-2'}`}
                {...props}
              >
                {children}
              </code>
            );
          },
          pre({ children }) {
            return <>{children}</>;
          },
          p({ children }) {
            return <p className="mb-2 last:mb-0">{children}</p>;
          },
          ul({ children }) {
            return <ul className="list-disc list-inside mb-2 space-y-1">{children}</ul>;
          },
          ol({ children }) {
            return <ol className="list-decimal list-inside mb-2 space-y-1">{children}</ol>;
          },
          li({ children }) {
            return <li className="text-sm">{children}</li>;
          },
          strong({ children }) {
            return <strong className="font-semibold text-slate-800">{children}</strong>;
          },
          em({ children }) {
            return <em className="italic">{children}</em>;
          },
          a({ href, children }) {
            return (
              <a href={href} target="_blank" rel="noopener noreferrer" className="text-indigo-600 hover:underline">
                {children}
              </a>
            );
          },
          blockquote({ children }) {
            return (
              <blockquote className="border-l-4 border-slate-200 pl-3 py-1 my-2 bg-slate-50 italic text-slate-600">
                {children}
              </blockquote>
            );
          },
          h1({ children }) {
            return <h1 className="text-lg font-bold mb-2">{children}</h1>;
          },
          h2({ children }) {
            return <h2 className="text-base font-bold mb-1.5">{children}</h2>;
          },
          h3({ children }) {
            return <h3 className="text-sm font-bold mb-1">{children}</h3>;
          },
        }}
      >
        {content}
      </ReactMarkdown>
    </div>
  );
}

function ToolCallPanel({ calls }: { calls: Array<{ id: string; function: { name: string; arguments: string } }> }) {
  const [expanded, setExpanded] = useState(false);

  if (!calls || calls.length === 0) return null;

  return (
    <div className="my-2">
      {calls.map((call) => (
        <div key={call.id} className="bg-violet-50 border border-violet-200 rounded-lg overflow-hidden">
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-violet-100 transition-colors"
          >
            <svg className="w-4 h-4 text-violet-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
            <span className="text-sm font-medium text-violet-800">{call.function.name}</span>
            <span className="text-xs text-violet-500 ml-auto">
              {expanded ? 'Hide' : 'Show'} arguments
            </span>
          </button>
          {expanded && (
            <div className="px-3 py-2 bg-violet-100 border-t border-violet-200">
              <pre className="text-xs font-mono text-violet-900 whitespace-pre-wrap break-all">
                {call.function.arguments}
              </pre>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

// StreamingToolCallPanel displays tool calls being built during streaming
function StreamingToolCallPanel({ calls, isStreaming }: { calls: Array<{ id?: string; name?: string; arguments: string }>; isStreaming?: boolean }) {
  const [expanded, setExpanded] = useState(true);

  if (!calls || calls.length === 0) return null;

  return (
    <div className="my-2">
      {calls.map((call, index) => (
        <div key={call.id || index} className="bg-violet-50 border border-violet-200 rounded-lg overflow-hidden">
          <button
            onClick={() => setExpanded(!expanded)}
            className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-violet-100 transition-colors"
          >
            <svg className="w-4 h-4 text-violet-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
            </svg>
            <span className="text-sm font-medium text-violet-800">{call.name || 'Unknown Tool'}</span>
            {isStreaming && (
              <span className="w-2 h-2 rounded-full bg-violet-400 animate-pulse" />
            )}
            <span className="text-xs text-violet-500 ml-auto">
              {expanded ? 'Hide' : 'Show'} arguments
            </span>
          </button>
          {expanded && (
            <div className="px-3 py-2 bg-violet-100 border-t border-violet-200">
              <pre className="text-xs font-mono text-violet-900 whitespace-pre-wrap break-all">
                {call.arguments || '...'}
              </pre>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

// SystemMeta displays system prompts meta info (collapsible)
function SystemMeta({ systemPrompts }: { systemPrompts?: string[] }) {
  const [expanded, setExpanded] = useState(false);

  if (!systemPrompts || systemPrompts.length === 0) return null;

  return (
    <div className="inline-flex items-start">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 text-xs hover:bg-amber-200 transition-colors"
      >
        <svg className={`w-3 h-3 transition-transform ${expanded ? 'rotate-90' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="font-medium">{systemPrompts.length} System</span>
      </button>

      {expanded && (
        <div className="ml-2 mt-0.5 p-2 bg-amber-50 border border-amber-200 rounded text-xs max-w-md">
          <div className="font-medium text-amber-700 mb-1">System Prompts</div>
          <SystemContent content={systemPrompts} />
        </div>
      )}
    </div>
  );
}

// ToolDefItem displays a single tool with expandable details
function ToolDefItem({ tool }: { tool: ToolDef }) {
  const [expanded, setExpanded] = useState(false);

  const toolDefinition = JSON.stringify(tool, null, 2);

  return (
    <div className="border border-violet-200 rounded overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full px-2 py-1 flex items-center gap-1 bg-violet-50 hover:bg-violet-100 transition-colors"
      >
        <svg className={`w-3 h-3 text-violet-500 transition-transform ${expanded ? 'rotate-90' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="font-medium text-violet-800 text-xs flex-1 text-left">{tool.name}</span>
      </button>
      {expanded && (
        <div className="px-2 py-1.5 bg-white text-xs space-y-2">
          {tool.description && (
            <div className="flex items-start justify-between gap-2">
              <div className="text-violet-700 flex-1"><MarkdownContent content={tool.description} /></div>
              <CopyButton text={tool.description} />
            </div>
          )}
          <div className="flex items-center justify-between">
            <div className="text-[10px] text-violet-500">Parameters</div>
            <CopyButton text={toolDefinition} />
          </div>
          <pre className="text-violet-700 bg-violet-50 p-1.5 rounded overflow-x-auto text-[10px] leading-relaxed">
            {JSON.stringify(tool.input_schema, null, 2)}
          </pre>
        </div>
      )}
    </div>
  );
}

// ToolsMeta displays tools meta info (collapsible)
function ToolsMeta({ tools }: { tools?: ToolDef[] }) {
  const [expanded, setExpanded] = useState(false);

  if (!tools || tools.length === 0) return null;

  return (
    <div className="inline-flex items-start">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-100 text-violet-700 text-xs hover:bg-violet-200 transition-colors"
      >
        <svg className={`w-3 h-3 transition-transform ${expanded ? 'rotate-90' : ''}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="font-medium">{tools.length} Tools</span>
      </button>

      {expanded && (
        <div className="ml-2 mt-0.5 p-2 bg-violet-50 border border-violet-200 rounded text-xs max-w-md">
          <div className="font-medium text-violet-700 mb-1">Tools</div>
          <div className="space-y-1">
            {tools.map((tool, i) => (
              <ToolDefItem key={i} tool={tool} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// UserMessageMeta displays system prompts and tools meta info for user messages
function UserMessageMeta({ systemPrompts, tools }: { systemPrompts?: string[]; tools?: ToolDef[] }) {
  const hasSystemPrompts = systemPrompts && systemPrompts.length > 0;
  const hasTools = tools && tools.length > 0;

  if (!hasSystemPrompts && !hasTools) return null;

  return (
    <div className="mt-2 space-y-2">
      <SystemMeta systemPrompts={systemPrompts} />
      <ToolsMeta tools={tools} />
    </div>
  );
}

// ThinkingContent displays Claude's thinking process
function ThinkingContent({ content, isStreaming }: { content: string; isStreaming?: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const [displayContent, setDisplayContent] = useState(content);

  // Update content when streaming
  useEffect(() => {
    if (isStreaming) {
      setDisplayContent(content);
    }
  }, [content, isStreaming]);

  if (!displayContent) return null;

  return (
    <div className="mt-2">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-1.5 px-2 py-1 rounded bg-slate-100 hover:bg-slate-200 transition-colors text-xs"
      >
        <svg
          className={`w-3.5 h-3.5 text-slate-500 transition-transform ${expanded ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
        <span className="font-medium text-slate-600">Thinking</span>
        {isStreaming && (
          <span className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
        )}
      </button>
      {expanded && (
        <div className="mt-1.5 px-3 py-2 bg-slate-50 border border-slate-200 rounded-lg">
          <pre className="text-xs font-mono text-slate-600 whitespace-pre-wrap break-all">
            {displayContent}
          </pre>
        </div>
      )}
    </div>
  );
}

export function MessageBubble({ role, content, isStreaming, tokens, tool_calls, streaming_tool_calls, timestamp, system_prompts, tools, thinking }: MessageBubbleProps) {
  const isUser = role === 'user';
  const isAssistant = role === 'assistant';

  const roleColors = {
    user: 'bg-indigo-50 border-indigo-200',
    assistant: 'bg-white border-slate-200',
    system: 'bg-amber-50 border-amber-200',
    tool: 'bg-violet-50 border-violet-200',
  };

  const roleIcon = {
    user: (
      <svg className="w-4 h-4 text-indigo-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
      </svg>
    ),
    assistant: (
      <svg className="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
      </svg>
    ),
    system: (
      <svg className="w-4 h-4 text-amber-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
      </svg>
    ),
    tool: (
      <svg className="w-4 h-4 text-violet-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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
          <span className={`text-xs font-medium ${isUser ? 'text-indigo-700' : 'text-slate-600'}`}>
            {roleLabel[role]}
          </span>
          {timestamp && (
            <span className="text-xs text-slate-400">{formatTime(timestamp)}</span>
          )}
          {isStreaming && (
            <span className="flex items-center gap-1 text-xs text-emerald-600">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              Streaming
            </span>
          )}
        </div>

        <div className={`rounded-xl p-4 border ${roleColors[role]} rounded-tl-sm`}>
          {/* Completed tool calls */}
          {tool_calls && tool_calls.length > 0 && (
            <ToolCallPanel calls={tool_calls} />
          )}
          {/* Streaming tool calls (being built) */}
          {streaming_tool_calls && streaming_tool_calls.length > 0 && (
            <StreamingToolCallPanel calls={streaming_tool_calls} isStreaming={isStreaming} />
          )}

          {/* Content */}
          <div className={`text-sm leading-relaxed ${isUser ? 'text-indigo-900' : 'text-slate-800'}`}>
            {/* System prompt uses special collapsible view */}
            {role === 'system' ? (
              <SystemContent content={Array.isArray(content) ? content : [String(content)]} />
            ) : Array.isArray(content) ? (
              <div className="space-y-2">
                {content.map((c, i) => (
                  <div key={i} className="pb-2 border-b border-slate-200 last:border-0 last:pb-0">
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

          {/* Thinking content (for Claude) */}
          {isAssistant && thinking && (
            <ThinkingContent content={thinking} isStreaming={isStreaming} />
          )}

          {/* Token count */}
          {tokens && tokens > 0 && (
            <div className="mt-2 pt-2 border-t border-slate-200/50 flex items-center gap-2 text-xs text-slate-400">
              <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
              </svg>
              <span>{tokens} tokens</span>
            </div>
          )}
        </div>

        {/* System/Tools meta for user messages */}
        {isUser && <UserMessageMeta systemPrompts={system_prompts} tools={tools} />}
      </div>
    </div>
  );
}
