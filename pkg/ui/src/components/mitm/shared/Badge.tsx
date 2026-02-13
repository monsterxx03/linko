import React from 'react';

export interface BadgeProps {
  children: React.ReactNode;
  colorClass: string;
  className?: string;
}

export function Badge({ children, colorClass, className = '' }: BadgeProps) {
  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colorClass} ${className}`}>
      {children}
    </span>
  );
}

// Method badge colors
export const METHOD_COLORS: Record<string, string> = {
  GET: 'bg-green-100 text-green-800',
  POST: 'bg-blue-100 text-blue-800',
  PUT: 'bg-yellow-100 text-yellow-800',
  DELETE: 'bg-red-100 text-red-800',
  PATCH: 'bg-purple-100 text-purple-800',
  HEAD: 'bg-gray-100 text-gray-800',
  OPTIONS: 'bg-indigo-100 text-indigo-800',
  CONNECT: 'bg-teal-100 text-teal-800',
};

// Status code colors - optimized per design doc
export const STATUS_COLORS: Record<number, string> = {
  2: 'bg-emerald-100 text-emerald-800 border-emerald-200',
  3: 'bg-blue-100 text-blue-800 border-blue-200',
  4: 'bg-amber-100 text-amber-800 border-amber-200',
  5: 'bg-red-100 text-red-800 border-red-200',
};

export function getMethodColor(method?: string): string {
  return METHOD_COLORS[method || ''] || 'bg-gray-100 text-gray-800 border-gray-200';
}

export function getStatusColor(status?: number): string {
  const statusCode = status || 0;
  if (statusCode === 0) return 'bg-gray-100 text-gray-800 border-gray-200';
  const category = Math.floor(statusCode / 100) as 2 | 3 | 4 | 5;
  return STATUS_COLORS[category] || 'bg-gray-100 text-gray-800 border-gray-200';
}
