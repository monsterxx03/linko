import { useState, useMemo, useCallback, useEffect, useRef } from 'react';
import { useTraffic } from '../hooks/useTraffic';
import { TrafficEvent } from '../contexts/SSEContext';
import { TrafficHeader, TrafficControls, TrafficItem } from '../components/mitm';

function MitmTraffic() {
  const { events, isConnected, error, search, setSearch, setAutoScroll, clear, reconnect } = useTraffic({ maxEvents: 100, autoScroll: true });
  const [collapseVer, setCollapseVer] = useState(0);
  const [bodyExpanded, setBodyExpanded] = useState(true);
  const listRef = useRef<HTMLDivElement>(null);

  const handleSearchChange = useCallback((value: string) => {
    setSearch(value);
  }, [setSearch]);

  const handleAutoScrollChange = useCallback((checked: boolean) => {
    setAutoScroll(checked);
  }, [setAutoScroll]);

  const handleCollapseBodies = useCallback(() => {
    setBodyExpanded(false);
    setCollapseVer(v => v + 1);
  }, []);

  const connectedClients = useMemo(() => {
    return events.length > 0 ? '1' : '0';
  }, [events.length]);

  // Auto scroll effect
  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [events.length]);

  return (
    <div className="tab-section">
      <TrafficHeader
        isConnected={isConnected}
        error={error}
        clientCount={parseInt(connectedClients, 10)}
        onCollapseBodies={handleCollapseBodies}
        onClear={clear}
        onReconnect={reconnect}
      />

      <TrafficControls
        search={search}
        autoScroll={true}
        onSearchChange={handleSearchChange}
        onAutoScrollChange={handleAutoScrollChange}
      />

      <div className="bg-white rounded-xl border border-bg-200 shadow-sm overflow-hidden">
        <div className="px-5 py-4 border-b border-bg-100 flex items-center justify-between bg-bg-50/50">
          <h2 className="font-semibold text-bg-800">MITM Traffic</h2>
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
            <span className="text-xs text-bg-400 font-medium">Live</span>
          </div>
        </div>
        <div ref={listRef} className="max-h-[600px] overflow-y-auto">
          <div className="p-4">
            {!isConnected && !error && (
              <div className="text-center py-12">
                <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-bg-100 mb-4">
                  <svg className="w-6 h-6 text-bg-400 animate-spin" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                  </svg>
                </div>
                <p className="text-bg-400">Connecting to SSE endpoint...</p>
              </div>
            )}
            {error && (
              <div className="text-center py-12">
                <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-red-50 mb-4">
                  <svg className="w-6 h-6 text-red-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                </div>
                <p className="text-red-500 font-medium">Error: {error}</p>
              </div>
            )}
            {isConnected && events.length === 0 && (
              <div className="text-center py-12">
                <div className="inline-flex items-center justify-center w-12 h-12 rounded-full bg-bg-100 mb-4">
                  <svg className="w-6 h-6 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                </div>
                <p className="text-bg-400">No traffic data available</p>
                <p className="text-xs text-bg-300 mt-1">Traffic will appear here when requests are captured</p>
              </div>
            )}
            {events.map((e: TrafficEvent) => (
              <TrafficItem key={`${e.id}-${collapseVer}`} event={e} bodyExpanded={bodyExpanded} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

export default MitmTraffic;
