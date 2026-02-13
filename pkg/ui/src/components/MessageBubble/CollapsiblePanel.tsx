import React, { useState, memo, useCallback } from 'react';
import { ChevronIcon } from './icons';

interface CollapsiblePanelProps {
  title: React.ReactNode;
  children: React.ReactNode;
  defaultExpanded?: boolean;
  expanded?: boolean;
  onToggle?: () => void;
  className?: string;
  buttonClassName?: string;
  contentClassName?: string;
  showChevron?: boolean;
  chevronSize?: 'sm' | 'md' | 'lg';
  chevronClassName?: string;
  headerRight?: React.ReactNode | ((expanded: boolean) => React.ReactNode);
  borderColor?: string;
  bgColor?: string;
}

export const CollapsiblePanel = memo(function CollapsiblePanel({
  title,
  children,
  defaultExpanded = false,
  expanded: controlledExpanded,
  onToggle,
  className = '',
  buttonClassName = '',
  contentClassName = '',
  showChevron = true,
  chevronSize = 'md',
  chevronClassName = 'text-slate-500',
  headerRight,
  borderColor = 'border-slate-200',
  bgColor = 'bg-slate-50',
}: CollapsiblePanelProps) {
  const [internalExpanded, setInternalExpanded] = useState(defaultExpanded);
  const expanded = controlledExpanded !== undefined ? controlledExpanded : internalExpanded;

  const handleToggle = useCallback(() => {
    if (onToggle) {
      onToggle();
    } else if (controlledExpanded === undefined) {
      setInternalExpanded(!expanded);
    }
  }, [onToggle, controlledExpanded, expanded]);

  return (
    <div className={`border ${borderColor} rounded-lg overflow-hidden my-2 ${className}`}>
      <button
        type="button"
        onClick={handleToggle}
        className={`w-full px-3 py-2 flex items-center gap-2 text-left transition-colors ${bgColor} ${buttonClassName}`}
      >
        {showChevron && (
          <ChevronIcon expanded={expanded} size={chevronSize} className={chevronClassName} />
        )}
        <div className="flex-1">{title}</div>
        {typeof headerRight === 'function' ? headerRight(expanded) : headerRight}
      </button>
      {expanded && (
        <div className={`px-3 py-2 ${bgColor} border-t ${borderColor} ${contentClassName}`}>
          {children}
        </div>
      )}
    </div>
  );
});
