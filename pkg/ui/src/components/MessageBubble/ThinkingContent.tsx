import { useState, useEffect, memo, useCallback } from 'react';
import { ChevronIcon } from './icons';

interface ThinkingContentProps {
  content: string;
  isStreaming?: boolean;
}

export const ThinkingContent = memo(function ThinkingContent({
  content,
  isStreaming,
}: ThinkingContentProps) {
  const [expanded, setExpanded] = useState(true);
  const [displayContent, setDisplayContent] = useState(content);

  useEffect(() => {
    if (isStreaming) {
      setDisplayContent(content);
    }
  }, [content, isStreaming]);

  if (!displayContent) return null;

  const handleToggle = useCallback(() => setExpanded(!expanded), [expanded]);

  return (
    <div className="mt-2">
      <button
        type="button"
        onClick={handleToggle}
        className="flex items-center gap-1.5 px-2 py-1 rounded bg-slate-100 hover:bg-slate-200 transition-colors text-xs"
      >
        <ChevronIcon expanded={expanded} size="md" className="text-slate-500 w-3.5 h-3.5" />
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
});
