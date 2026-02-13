import { useState, memo, useCallback } from 'react';
import { ChevronIcon } from './icons';
import { SystemContent } from './SystemContent';

interface SystemMetaProps {
  systemPrompts?: string[];
}

export const SystemMeta = memo(function SystemMeta({ systemPrompts }: SystemMetaProps) {
  const [expanded, setExpanded] = useState(false);

  const handleToggle = useCallback(() => setExpanded((prev) => !prev), []);

  if (!systemPrompts || systemPrompts.length === 0) return null;

  return (
    <div className="inline-flex items-start">
      <button
        type="button"
        onClick={handleToggle}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 text-xs hover:bg-amber-200 transition-colors"
      >
        <ChevronIcon expanded={expanded} size="sm" className="text-amber-600" />
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
});
