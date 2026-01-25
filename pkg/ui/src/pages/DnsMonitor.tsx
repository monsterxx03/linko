import { useState, useEffect, useCallback } from 'react';
import {
  fetchStats,
  fetchLatency,
  clearStats,
  clearCache,
  formatNumber,
  formatPercent,
  DNSStats,
  CacheStats,
} from '../utils/api';

function DnsMonitor() {
  const [latency, setLatency] = useState('--ms');
  const [dnsStats, setDnsStats] = useState<DNSStats | null>(null);
  const [cacheStats, setCacheStats] = useState<CacheStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [feedback, setFeedback] = useState<{ message: string; type: 'success' | 'error' } | null>(null);

  const refreshData = useCallback(async () => {
    const lat = await fetchLatency();
    setLatency(lat);

    const data = await fetchStats();
    if (data) {
      setDnsStats(data.data.dns);
      setCacheStats(data.data.cache);
    }
    setLoading(false);
  }, []);

  const showFeedback = useCallback((message: string, type: 'success' | 'error') => {
    setFeedback({ message, type });
    setTimeout(() => setFeedback(null), 3000);
  }, []);

  const handleClearStats = useCallback(async () => {
    const success = await clearStats();
    if (success) {
      showFeedback('Stats cleared', 'success');
      await refreshData();
    } else {
      showFeedback('Failed to clear stats', 'error');
    }
  }, [refreshData, showFeedback]);

  const handleClearCache = useCallback(async () => {
    const success = await clearCache();
    if (success) {
      showFeedback('Cache cleared', 'success');
      await refreshData();
    } else {
      showFeedback('Failed to clear cache', 'error');
    }
  }, [refreshData, showFeedback]);

  const handleClearBoth = useCallback(async () => {
    await handleClearStats();
    await handleClearCache();
  }, [handleClearStats, handleClearCache]);

  useEffect(() => {
    refreshData();
    const interval = setInterval(refreshData, 5000);
    return () => clearInterval(interval);
  }, [refreshData]);

  return (
    <div className="tab-section">
      {/* Status Bar */}
      <div className="bg-white rounded-xl border border-bg-200 p-4 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2">
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500" />
              <span className="text-sm font-medium text-bg-800">DNS Server</span>
              <span className="text-sm text-emerald-600">Online</span>
            </div>
            <div className="h-5 w-px bg-bg-200" />
            <div className="text-sm text-bg-600">
              API latency: <span className="font-medium text-bg-800">{latency}</span>
            </div>
          </div>
          <button
            onClick={refreshData}
            className="btn-action px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200"
          >
            Refresh
          </button>
        </div>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        {/* Total Queries */}
        <div className="stat-card bg-white rounded-xl border border-bg-200 p-5">
          <p className="text-xs font-medium text-bg-500 uppercase tracking-wide mb-1">Total Queries</p>
          <p className="text-2xl font-semibold text-bg-900 stat-value">
            {loading ? '--' : formatNumber(dnsStats?.total_queries)}
          </p>
          <p className="text-xs text-bg-400 mt-1">Since last reset</p>
        </div>

        {/* Success Rate */}
        <div className="stat-card bg-white rounded-xl border border-bg-200 p-5">
          <p className="text-xs font-medium text-bg-500 uppercase tracking-wide mb-1">Success Rate</p>
          <p className="text-2xl font-semibold text-emerald-600 stat-value">
            {loading ? '--%' : formatPercent(dnsStats?.success_rate)}
          </p>
          <div className="mt-2 h-1.5 bg-bg-100 rounded-full overflow-hidden">
            <div
              className="h-full bg-emerald-500 transition-all duration-500"
              style={{ width: `${dnsStats?.success_rate || 0}%` }}
            />
          </div>
        </div>

        {/* Avg Response */}
        <div className="stat-card bg-white rounded-xl border border-bg-200 p-5">
          <p className="text-xs font-medium text-bg-500 uppercase tracking-wide mb-1">Avg Response</p>
          <p className="text-2xl font-semibold text-bg-800 stat-value">
            {loading ? '--ms' : dnsStats?.avg_response_time
              ? `${Math.floor(parseFloat(dnsStats.avg_response_time.replace('ms', '')))}ms`
              : '--ms'}
          </p>
          <p className="text-xs text-bg-400 mt-1">Last query average</p>
        </div>

        {/* Cache Size */}
        <div className="stat-card bg-white rounded-xl border border-bg-200 p-5">
          <p className="text-xs font-medium text-bg-500 uppercase tracking-wide mb-1">Cache Size</p>
          <p className="text-2xl font-semibold text-bg-800 stat-value">
            {loading ? '--' : formatNumber(cacheStats?.size)}
          </p>
          <p className="text-xs text-bg-400 mt-1">
            Hit rate: <span className="text-emerald-600">{formatPercent(cacheStats?.hit_rate)}</span>
          </p>
        </div>
      </div>

      {/* Secondary Stats */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white rounded-xl border border-bg-200 p-4 flex items-center justify-between">
          <div>
            <p className="text-xs text-bg-500 mb-1">Domains</p>
            <p className="text-lg font-semibold text-bg-800 stat-value">
              {loading ? '--' : formatNumber(dnsStats?.total_domains)}
            </p>
          </div>
          <div className="w-8 h-8 rounded-lg bg-bg-100 flex items-center justify-center">
            <svg className="w-4 h-4 text-bg-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064"
              />
            </svg>
          </div>
        </div>

        <div className="bg-white rounded-xl border border-bg-200 p-4 flex items-center justify-between">
          <div>
            <p className="text-xs text-bg-500 mb-1">Success</p>
            <p className="text-lg font-semibold text-emerald-600 stat-value">
              {loading ? '--' : formatNumber(dnsStats?.total_success)}
            </p>
          </div>
          <div className="w-8 h-8 rounded-lg bg-emerald-50 flex items-center justify-center">
            <svg className="w-4 h-4 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
            </svg>
          </div>
        </div>

        <div className="bg-white rounded-xl border border-bg-200 p-4 flex items-center justify-between">
          <div>
            <p className="text-xs text-bg-500 mb-1">Failed</p>
            <p className="text-lg font-semibold text-red-500 stat-value">
              {loading ? '--' : formatNumber(dnsStats?.total_failed)}
            </p>
          </div>
          <div className="w-8 h-8 rounded-lg bg-red-50 flex items-center justify-center">
            <svg className="w-4 h-4 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </div>
        </div>

        <div className="bg-white rounded-xl border border-bg-200 p-4 flex items-center justify-between">
          <div>
            <p className="text-xs text-bg-500 mb-1">Cache Items</p>
            <p className="text-lg font-semibold text-bg-800 stat-value">
              {loading ? '--' : formatNumber(cacheStats?.size)}
            </p>
          </div>
          <div className="w-8 h-8 rounded-lg bg-bg-100 flex items-center justify-center">
            <svg className="w-4 h-4 text-bg-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M4 7v10c0 2.21 3.582 4 8 4s8-1.79 8-4V7M4 7c0 2.21 3.582 4 8 4s8-1.79 8-4M4 7c0-2.21 3.582-4 8-4s8 1.79 8 4"
              />
            </svg>
          </div>
        </div>
      </div>

      {/* Top Domains Table */}
      <div className="bg-white rounded-xl border border-bg-200 mb-6">
        <div className="px-5 py-4 border-b border-bg-100 flex items-center justify-between">
          <h2 className="font-semibold text-bg-800">Top Domains</h2>
          <span className="text-xs text-bg-400">Live</span>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="text-xs font-medium text-bg-500 uppercase tracking-wide border-b border-bg-100">
                <th className="px-5 py-3 text-left">#</th>
                <th className="px-5 py-3 text-left">Domain</th>
                <th className="px-5 py-3 text-left">Type</th>
                <th className="px-5 py-3 text-right">Queries</th>
                <th className="px-5 py-3 text-right">Share</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                <tr className="domain-row border-t border-bg-100">
                  <td colSpan={5} className="px-5 py-8 text-center text-bg-400">
                    <div className="h-4 bg-bg-100 rounded w-48 mx-auto animate-pulse" />
                  </td>
                </tr>
              ) : !dnsStats?.top_domains || dnsStats.top_domains.length === 0 ? (
                <tr className="domain-row border-t border-bg-100">
                  <td colSpan={5} className="px-5 py-8 text-center text-bg-400">
                    No data available
                  </td>
                </tr>
              ) : (
                dnsStats.top_domains.map((domain, index) => {
                  const totalQueries = dnsStats.top_domains!.reduce((sum, d) => sum + d.total_queries, 0);
                  const percent = totalQueries > 0 ? (domain.total_queries / totalQueries * 100).toFixed(1) : 0;
                  const queryTypes = domain.query_types || {};
                  const primaryType = Object.keys(queryTypes)[0] || 'A';
                  const rankClass = index < 3 ? 'bg-bg-800 text-white' : 'bg-bg-100 text-bg-600';

                  return (
                    <tr key={domain.domain} className="domain-row border-t border-bg-100">
                      <td className="px-5 py-3">
                        <span className={`inline-flex items-center justify-center w-6 h-6 rounded text-xs font-medium ${rankClass}`}>
                          {index + 1}
                        </span>
                      </td>
                      <td className="px-5 py-3 text-sm text-bg-700 truncate max-w-xs" title={domain.domain}>
                        {domain.domain}
                      </td>
                      <td className="px-5 py-3">
                        <span className="inline-flex px-2 py-0.5 rounded text-xs font-medium bg-bg-100 text-bg-600">
                          {primaryType}
                        </span>
                      </td>
                      <td className="px-5 py-3 text-right text-sm font-medium text-bg-800">
                        {formatNumber(domain.total_queries)}
                      </td>
                      <td className="px-5 py-3 text-right">
                        <div className="flex items-center justify-end gap-2">
                          <div className="w-12 h-1.5 bg-bg-100 rounded-full overflow-hidden">
                            <div className="h-full bg-bg-300 transition-all" style={{ width: `${percent}%` }} />
                          </div>
                          <span className="text-xs text-bg-400 w-10 text-right">{percent}%</span>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Actions */}
      <div className="bg-white rounded-xl border border-bg-200 p-5">
        <h2 className="font-semibold text-bg-800 mb-4">Actions</h2>
        <div className="flex flex-wrap gap-3">
          <button
            onClick={handleClearStats}
            className="btn-action px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200"
          >
            Clear Stats
          </button>
          <button
            onClick={handleClearCache}
            className="btn-action px-4 py-2 text-sm font-medium text-bg-700 bg-bg-100 rounded-lg hover:bg-bg-200"
          >
            Clear Cache
          </button>
          <button
            onClick={handleClearBoth}
            className="btn-action px-4 py-2 text-sm font-medium text-red-600 bg-red-50 rounded-lg hover:bg-red-100"
          >
            Clear All
          </button>
        </div>
        {feedback && (
          <div className={`mt-3 text-sm animate-fade-in ${feedback.type === 'success' ? 'text-emerald-600' : 'text-red-500'}`}>
            {feedback.message}
          </div>
        )}
      </div>
    </div>
  );
}

export default DnsMonitor;
