import React, { useCallback } from 'react';

export interface TrafficControlsProps {
  search: string;
  autoScroll: boolean;
  onSearchChange: (value: string) => void;
  onAutoScrollChange: (checked: boolean) => void;
}

export function TrafficControls({
  search,
  autoScroll,
  onSearchChange,
  onAutoScrollChange,
}: TrafficControlsProps) {
  const handleSearchChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    onSearchChange(e.target.value);
  }, [onSearchChange]);

  const handleAutoScrollChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    onAutoScrollChange(e.target.checked);
  }, [onAutoScrollChange]);

  return (
    <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6 shadow-sm">
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-2 flex-1 min-w-[200px]">
          <svg className="w-4 h-4 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
          </svg>
          <input
            type="text"
            value={search}
            onChange={handleSearchChange}
            placeholder="Search URLs, domains..."
            className="flex-1 px-3 py-1.5 border border-bg-300 rounded-lg text-sm focus:ring-2 focus:ring-accent-500 focus:border-accent-500 outline-none transition-all duration-150"
          />
          {search && (
            <button
              onClick={() => onSearchChange('')}
              className="text-bg-400 hover:text-bg-600 p-1"
              title="Clear search"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
        <div className="flex items-center gap-2 ml-auto">
          <input
            type="checkbox"
            id="auto-scroll"
            checked={autoScroll}
            onChange={handleAutoScrollChange}
            className="rounded text-accent-500 focus:ring-accent-500 focus:ring-offset-0"
          />
          <label htmlFor="auto-scroll" className="text-sm text-bg-600 cursor-pointer select-none">
            Auto scroll
          </label>
        </div>
      </div>
    </div>
  );
}
