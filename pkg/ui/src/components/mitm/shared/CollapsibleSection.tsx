import React, { useState, useCallback } from 'react';

export interface CollapsibleSectionProps {
  title: string;
  defaultExpanded?: boolean;
  children: React.ReactNode;
}

export function CollapsibleSection({ title, defaultExpanded = false, children }: CollapsibleSectionProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const toggleExpanded = useCallback(() => {
    setExpanded(prev => !prev);
  }, []);

  return (
    <div className="p-3 bg-bg-50 rounded-lg border border-bg-100">
      <button
        onClick={toggleExpanded}
        className="flex items-center justify-between w-full text-left hover:bg-bg-100 rounded px-2 py-1.5 -mx-2 -mt-1 transition-colors duration-150"
      >
        <span className="text-xs font-medium text-bg-600">{title}</span>
        <svg
          className={`w-4 h-4 text-bg-400 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>
      {expanded && <div className="mt-2">{children}</div>}
    </div>
  );
}
