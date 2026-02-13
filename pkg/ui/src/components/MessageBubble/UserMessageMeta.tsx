import { memo } from 'react';
import { SystemMeta } from './SystemMeta';
import { ToolsMeta, ToolDef } from './ToolsMeta';

interface UserMessageMetaProps {
  systemPrompts?: string[];
  tools?: ToolDef[];
}

export const UserMessageMeta = memo(function UserMessageMeta({
  systemPrompts,
  tools,
}: UserMessageMetaProps) {
  const hasSystemPrompts = systemPrompts && systemPrompts.length > 0;
  const hasTools = tools && tools.length > 0;

  if (!hasSystemPrompts && !hasTools) return null;

  return (
    <div className="mt-2 space-y-2">
      <SystemMeta systemPrompts={systemPrompts} />
      <ToolsMeta tools={tools} />
    </div>
  );
});
