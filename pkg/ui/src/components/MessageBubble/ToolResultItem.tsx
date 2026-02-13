import { memo } from 'react';
import { CollapsiblePanel } from './CollapsiblePanel';

interface ToolResultItemProps {
  result: { tool_use_id: string; content: string };
}

const STRINGS = {
  EXPAND: 'Expand',
  COLLAPSE: 'Collapse',
} as const;

export const ToolResultItem = memo(function ToolResultItem({
  result,
}: ToolResultItemProps) {
  return (
    <CollapsiblePanel
      title={
        <div className="flex flex-col flex-1">
          <span className="text-xs text-violet-400 font-mono">
            {result.tool_use_id}
          </span>
        </div>
      }
      headerRight={(expanded) => (
        <span className="text-xs text-violet-500">
          {expanded ? STRINGS.COLLAPSE : STRINGS.EXPAND}
        </span>
      )}
      className="my-1"
      borderColor="border-violet-200"
      bgColor="bg-violet-50"
      buttonClassName="hover:bg-violet-100"
      contentClassName="bg-violet-100"
      chevronClassName="text-violet-500"
    >
      <pre className="text-sm text-violet-900 whitespace-pre-wrap break-all">
        {result.content}
      </pre>
    </CollapsiblePanel>
  );
});
