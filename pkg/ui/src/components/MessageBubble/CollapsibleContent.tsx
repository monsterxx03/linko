import { useState, memo, useCallback } from 'react';
import { hasSystemReminderTag } from './utils';

interface CollapsibleContentProps {
  content: string;
  index: number;
}

export const CollapsibleContent = memo(function CollapsibleContent({ content, index }: CollapsibleContentProps) {
  const hasTags = hasSystemReminderTag(content);

  if (!hasTags) {
    return <CollapsibleMarkdown content={content} index={index} />;
  }

  const tagName = 'system-reminder';
  const [collapsed, setCollapsed] = useState(false);

  const handleToggle = useCallback(() => setCollapsed(!collapsed), [collapsed]);

  return (
    <div className="border border-slate-200 rounded-lg overflow-hidden my-2">
      <button
        type="button"
        onClick={handleToggle}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-slate-50 transition-colors bg-slate-50"
      >
        <svg
          className={`w-4 h-4 text-slate-500 transition-transform ${collapsed ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M9 5l7 7-7 7"
          />
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
});

const COLLAPSE_THRESHOLD = 500;

interface CollapsibleMarkdownProps {
  content: string;
  index: number;
}

const CollapsibleMarkdown = memo(function CollapsibleMarkdown({ content, index }: CollapsibleMarkdownProps) {
  const [collapsed, setCollapsed] = useState(content.length > COLLAPSE_THRESHOLD);

  const handleToggle = useCallback(() => setCollapsed(!collapsed), [collapsed]);

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
        type="button"
        onClick={handleToggle}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-slate-50 transition-colors bg-slate-50"
      >
        <svg
          className={`w-4 h-4 text-slate-500 transition-transform ${collapsed ? 'rotate-90' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            strokeWidth={2}
            d="M9 5l7 7-7 7"
          />
        </svg>
        <span className="text-xs text-slate-500 font-mono flex-1 text-left">
          [{index}] {preview}
        </span>
        <span className="text-xs text-slate-400 whitespace-nowrap">
          {collapsed ? 'Expand' : 'Collapse'}
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
});
