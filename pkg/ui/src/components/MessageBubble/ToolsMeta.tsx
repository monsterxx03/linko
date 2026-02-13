import { useState, memo, useCallback } from 'react';
import { ChevronIcon } from './icons';
import { CopyButton } from './CopyButton';
import { MarkdownContent } from './MarkdownContent';

export interface ToolDef {
  name: string;
  description?: string;
  input_schema: Record<string, unknown>;
}

interface ToolDefItemProps {
  tool: ToolDef;
}

export const ToolDefItem = memo(function ToolDefItem({ tool }: ToolDefItemProps) {
  const [expanded, setExpanded] = useState(false);

  const handleToggle = useCallback(() => setExpanded(!expanded), []);

  const toolDefinition = JSON.stringify(tool, null, 2);

  return (
    <div className="border border-violet-200 rounded overflow-hidden">
      <button
        type="button"
        onClick={handleToggle}
        className="w-full px-2 py-1 flex items-center gap-1 bg-violet-50 hover:bg-violet-100 transition-colors"
      >
        <ChevronIcon expanded={expanded} size="sm" className="text-violet-500" />
        <span className="font-medium text-violet-800 text-xs flex-1 text-left">
          {tool.name}
        </span>
      </button>
      {expanded && (
        <div className="px-2 py-1.5 bg-white text-xs space-y-2">
          {tool.description && (
            <div className="flex items-start justify-between gap-2">
              <div className="text-violet-700 flex-1">
                <MarkdownContent content={tool.description} />
              </div>
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
});

interface ToolsMetaProps {
  tools?: ToolDef[];
}

export const ToolsMeta = memo(function ToolsMeta({ tools }: ToolsMetaProps) {
  const [expanded, setExpanded] = useState(false);

  const handleToggle = useCallback(() => setExpanded(!expanded), []);

  if (!tools || tools.length === 0) return null;

  return (
    <div className="inline-flex items-start">
      <button
        type="button"
        onClick={handleToggle}
        className="flex items-center gap-1 px-1.5 py-0.5 rounded bg-violet-100 text-violet-700 text-xs hover:bg-violet-200 transition-colors"
      >
        <ChevronIcon expanded={expanded} size="sm" className="text-violet-600" />
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
});
