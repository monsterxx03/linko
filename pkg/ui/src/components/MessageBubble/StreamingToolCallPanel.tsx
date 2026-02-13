import { useState, memo, useCallback } from 'react';
import { ToolIcon } from './icons';

interface StreamingToolCallPanelProps {
  calls: Array<{ id?: string; name?: string; arguments: string }>;
  isStreaming?: boolean;
}

export const StreamingToolCallPanel = memo(function StreamingToolCallPanel({
  calls,
  isStreaming,
}: StreamingToolCallPanelProps) {
  const [expanded, setExpanded] = useState(true);

  const handleToggle = useCallback(() => setExpanded(!expanded), [expanded]);

  if (!calls || calls.length === 0) return null;

  return (
    <div className="my-2">
      {calls.map((call, index) => (
        <div
          key={call.id || index}
          className="bg-violet-50 border border-violet-200 rounded-lg overflow-hidden"
        >
          <button
            type="button"
            onClick={handleToggle}
            className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-violet-100 transition-colors"
          >
            <ToolIcon />
            <span className="text-sm font-medium text-violet-800">
              {call.name || 'Unknown Tool'}
            </span>
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
});
