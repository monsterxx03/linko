import { memo } from 'react';
import { isEmptyContent, formatTime } from './utils';
import { ToolCallPanel } from './ToolCallPanel';
import { ToolResultItem } from './ToolResultItem';
import { StreamingToolCallPanel } from './StreamingToolCallPanel';
import { UserMessageMeta } from './UserMessageMeta';
import { ThinkingContent } from './ThinkingContent';
import { CollapsibleContent } from './CollapsibleContent';
import { SystemContent } from './SystemContent';

export interface MessageBubbleProps {
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
  tool_results?: Array<{
    tool_use_id: string;
    content: string;
  }>;
  timestamp?: number;
  system_prompts?: string[];
  tools?: Array<{
    name: string;
    description?: string;
    input_schema: Record<string, unknown>;
  }>;
  thinking?: string;
  id?: string;
}

const ROLE_CONFIG = {
  user: {
    colors: 'bg-indigo-50 border-indigo-200',
    label: 'You',
    contentColor: 'text-indigo-900',
  },
  assistant: {
    colors: 'bg-white border-slate-200',
    label: 'Assistant',
    contentColor: 'text-slate-800',
  },
  system: {
    colors: 'bg-amber-50 border-amber-200',
    label: 'System',
    contentColor: 'text-slate-800',
  },
  tool: {
    colors: 'bg-violet-50 border-violet-200',
    label: 'Tool',
    contentColor: 'text-slate-800',
  },
} as const;

const ROLE_ICONS = {
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

export const MessageBubble = memo(function MessageBubble({
  role,
  content,
  isStreaming,
  tokens,
  tool_calls,
  streaming_tool_calls,
  tool_results,
  timestamp,
  system_prompts,
  tools,
  thinking,
  id,
}: MessageBubbleProps) {
  const isUser = role === 'user';
  const isAssistant = role === 'assistant';

  const config = ROLE_CONFIG[role];
  const roleIcon = ROLE_ICONS[role];

  const shouldShowContent = !(
    isUser &&
    isEmptyContent(content) &&
    (!tool_calls || tool_calls.length === 0) &&
    (!streaming_tool_calls || streaming_tool_calls.length === 0)
  );

  const shouldShowBubble =
    (tool_calls && tool_calls.length > 0) ||
    (streaming_tool_calls && streaming_tool_calls.length > 0) ||
    shouldShowContent ||
    (thinking && thinking.trim().length > 0) ||
    (tokens && tokens > 0);

  const renderContent = () => {
    if (role === 'system') {
      return (
        <SystemContent
          content={Array.isArray(content) ? content : [String(content)]}
        />
      );
    }

    if (Array.isArray(content)) {
      return (
        <div className="space-y-2">
          {content.map((c, i) => (
            <div
              key={i}
              className="pb-2 border-b border-slate-200 last:border-0 last:pb-0"
            >
              <CollapsibleContent content={c} index={i} />
            </div>
          ))}
        </div>
      );
    }

    return <CollapsibleContent content={content} index={0} />;
  };

  return (
    <div className="flex gap-3">
      <div className={`flex-shrink-0 w-8 h-8 rounded-full flex items-center justify-center ${config.colors.replace('border', 'bg')}`}>
        {roleIcon}
      </div>

      <div className="flex-1 max-w-[80%]">
        <div className="flex items-center gap-2 mb-1">
          <span className={`text-xs font-medium ${isUser ? 'text-indigo-700' : 'text-slate-600'}`}>
            {config.label}
          </span>
          {id && (
            <span className="text-xs text-slate-400 font-mono" title={id}>
              {id}
            </span>
          )}
          {timestamp && (
            <span className="text-xs text-slate-400">
              {formatTime(timestamp)}
            </span>
          )}
          {isStreaming && (
            <span className="flex items-center gap-1 text-xs text-emerald-600">
              <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
              Streaming
            </span>
          )}
        </div>

        {shouldShowBubble && (
          <div className={`rounded-xl p-4 border ${config.colors} rounded-tl-sm`}>
            {tool_calls && tool_calls.length > 0 && (
              <ToolCallPanel calls={tool_calls} />
            )}

            {streaming_tool_calls && streaming_tool_calls.length > 0 && (
              <StreamingToolCallPanel
                calls={streaming_tool_calls}
                isStreaming={isStreaming}
              />
            )}

            {shouldShowContent && (
              <div className={`text-sm leading-relaxed ${config.contentColor}`}>
                {renderContent()}
                {isStreaming && (
                  <span className="inline-block w-2 h-4 ml-1 bg-emerald-500 animate-pulse" />
                )}
              </div>
            )}

            {isAssistant && thinking && (
              <ThinkingContent content={thinking} isStreaming={isStreaming} />
            )}

            {tokens && tokens > 0 && (
              <div className="mt-2 pt-2 border-t border-slate-200/50 flex items-center gap-2 text-xs text-slate-400">
                <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
                </svg>
                <span>{tokens} tokens</span>
              </div>
            )}
          </div>
        )}

        {isUser && (
          <>
            {tool_results && tool_results.length > 0 && (
              <div className="mt-2 space-y-1">
                {tool_results.map((result, i) => (
                  <ToolResultItem key={i} result={result} />
                ))}
              </div>
            )}
            <UserMessageMeta systemPrompts={system_prompts} tools={tools} />
          </>
        )}
      </div>
    </div>
  );
});
