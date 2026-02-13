import { useMemo } from 'react';

export interface TrafficHeaderProps {
  isConnected: boolean;
  error: string | null;
  clientCount: number;
  onCollapseBodies: () => void;
  onClear: () => void;
  onReconnect: () => void;
}

export function TrafficHeader({
  isConnected,
  error,
  clientCount,
  onCollapseBodies,
  onClear,
  onReconnect,
}: TrafficHeaderProps) {
  const connectionStatus = useMemo(() => {
    if (isConnected) {
      return { text: 'Online', color: 'text-emerald-600', dotColor: 'bg-emerald-500' };
    }
    return { text: 'Connecting...', color: 'text-red-500', dotColor: 'bg-red-500 animate-pulse' };
  }, [isConnected]);

  return (
    <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6 shadow-sm">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-6">
          <div className="flex items-center gap-3">
            <div className={`w-2.5 h-2.5 rounded-full ${connectionStatus.dotColor} ring-2 ring-emerald-500/20`} />
            <span className="text-sm font-semibold text-bg-800">MITM Proxy</span>
            <span className={`text-sm font-medium ${connectionStatus.color}`}>{connectionStatus.text}</span>
          </div>
          <div className="h-5 w-px bg-bg-200" />
          <div className="flex items-center gap-2">
            <svg className="w-4 h-4 text-bg-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
            <span className="text-sm text-bg-600">
              Connected clients: <span className="font-semibold text-bg-800">{clientCount}</span>
            </span>
          </div>
        </div>
        <div className="flex gap-2.5">
          <button
            onClick={onCollapseBodies}
            className="px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200 active:bg-bg-300 transition-colors duration-150"
          >
            Collapse Bodies
          </button>
          <button
            onClick={onClear}
            className="px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200 active:bg-bg-300 transition-colors duration-150"
          >
            Clear Traffic
          </button>
          {error && (
            <button
              onClick={onReconnect}
              className="px-4 py-2 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100 active:bg-red-200 transition-colors duration-150"
            >
              Reconnect
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
