import { useState, memo, useCallback } from 'react';
import { ToolIcon } from './icons';
import { formatToolCall } from './utils';

interface ToolCallPanelProps {
  calls: Array<{ id: string; function: { name: string; arguments: string } }>;
}

export const ToolCallPanel = memo(function ToolCallPanel({ calls }: ToolCallPanelProps) {
  const [expanded, setExpanded] = useState(false);

  const handleToggle = useCallback(() => setExpanded(!expanded), [expanded]);

  if (!calls || calls.length === 0) return null;

  return (
    <div className="my-2">
      {calls.map((call) => (
        <div
          key={call.id}
          className="bg-violet-50 border border-violet-200 rounded-lg overflow-hidden"
        >
          <button
            type="button"
            onClick={handleToggle}
            className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-violet-100 transition-colors"
          >
            <ToolIcon />
            <div className="flex flex-col">
              <span className="text-sm font-medium text-violet-800">
                {formatToolCall(call)}
              </span>
              <span className="text-[10px] text-violet-400 font-mono">
                {call.id}
              </span>
            </div>
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
});
