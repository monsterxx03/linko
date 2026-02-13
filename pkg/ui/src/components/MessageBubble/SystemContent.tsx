import { useState, memo } from 'react';
import { CopyButton } from './CopyButton';
import { MarkdownContent } from './MarkdownContent';

interface SingleSystemPromptProps {
  content: string;
}

const PREVIEW_LENGTH = 100;

const SingleSystemPrompt = memo(function SingleSystemPrompt({ content }: SingleSystemPromptProps) {
  const [collapsed, setCollapsed] = useState(true);
  const shouldCollapse = content.length > PREVIEW_LENGTH;

  if (!shouldCollapse) {
    return (
      <div className="flex items-start gap-1 px-2 py-1 group">
        <div className="text-xs text-slate-600 flex-1">
          <MarkdownContent content={content} />
        </div>
        <CopyButton
          text={content}
          className="opacity-0 group-hover:opacity-100"
        />
      </div>
    );
  }

  const preview = content.substring(0, PREVIEW_LENGTH) + '...';

  return (
    <>
      <button
        type="button"
        onClick={() => setCollapsed(!collapsed)}
        className="w-full px-3 py-2 flex items-center gap-2 text-left hover:bg-amber-100 transition-colors"
      >
        <svg
          className={`w-3 h-3 text-amber-600 transition-transform ${collapsed ? '' : 'rotate-90'}`}
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
        <span className="text-xs text-amber-700 flex-1">
          {collapsed ? preview : ''}
        </span>
        <span className="text-xs text-amber-500 whitespace-nowrap">
          {collapsed ? 'Expand' : 'Collapse'}
        </span>
      </button>
      {!collapsed && (
        <div className="px-3 py-2 bg-amber-50 border-t border-amber-200">
          <div className="flex items-start gap-1 group">
            <div className="text-xs text-slate-700 flex-1">
              <MarkdownContent content={content} />
            </div>
            <CopyButton text={content} />
          </div>
        </div>
      )}
    </>
  );
});

interface SystemContentProps {
  content: string[];
}

export const SystemContent = memo(function SystemContent({ content }: SystemContentProps) {
  if (content.length === 0) {
    return null;
  }

  if (content.length === 1) {
    return <SingleSystemPrompt content={content[0]} />;
  }

  return (
    <div className="space-y-3">
      {content.map((c, i) => (
        <div key={i} className="relative pl-6">
          {i > 0 && (
            <div className="absolute left-2 top-[-12px] w-0.5 h-3 bg-amber-200" />
          )}
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
});
