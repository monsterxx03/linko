import { memo } from 'react';

interface ChevronIconProps {
  expanded: boolean;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

const SIZE_CLASSES = {
  sm: 'w-3 h-3',
  md: 'w-4 h-4',
  lg: 'w-5 h-5',
} as const;

export const ChevronIcon = memo(function ChevronIcon({
  expanded,
  className = '',
  size = 'md',
}: ChevronIconProps) {
  return (
    <svg
      className={`${SIZE_CLASSES[size]} transition-transform ${expanded ? 'rotate-90' : ''} ${className}`}
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
  );
});

interface ToolIconProps {
  className?: string;
  size?: 'sm' | 'md' | 'lg';
}

export const ToolIcon = memo(function ToolIcon({ className = '', size = 'md' }: ToolIconProps) {
  return (
    <svg
      className={`${SIZE_CLASSES[size]} text-violet-500 ${className}`}
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth={2}
        d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4"
      />
    </svg>
  );
});
