import { useState, useEffect } from 'react';
import DnsMonitor from './pages/DnsMonitor';
import MitmTraffic from './pages/MitmTraffic';
import Conversations from './pages/Conversations';

type Tab = 'dns' | 'mitm' | 'conversations';

const TAB_VALUES: Tab[] = ['dns', 'mitm', 'conversations'];

function getTabFromHash(): Tab {
  const hash = window.location.hash.slice(1);
  return TAB_VALUES.includes(hash as Tab) ? (hash as Tab) : 'dns';
}

function App() {
  const [activeTab, setActiveTab] = useState<Tab>(getTabFromHash);

  // 从 URL hash 同步状态
  useEffect(() => {
    const handleHashChange = () => {
      setActiveTab(getTabFromHash());
    };
    window.addEventListener('hashchange', handleHashChange);
    return () => window.removeEventListener('hashchange', handleHashChange);
  }, []);

  const switchTab = (tab: Tab) => {
    setActiveTab(tab);
    window.location.hash = tab;
  };

  return (
    <div className="min-h-screen bg-bg-50">
      {/* Header */}
      <header className="bg-white border-b border-bg-200 sticky top-0 z-10">
        <div className="max-w-6xl mx-auto px-6 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-bg-800 flex items-center justify-center">
                <svg className="w-4 h-4 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
              </div>
              <div>
                <h1 className="text-lg font-semibold text-bg-900">Linko</h1>
                <p className="text-xs text-bg-500">Proxy & Monitor</p>
              </div>
            </div>

            <div className="flex items-center gap-4 text-sm">
              <div className="flex items-center gap-2 text-bg-600">
                <div className="w-2 h-2 rounded-full bg-emerald-500" />
                <span>Connected</span>
              </div>
              <span className="text-bg-400">|</span>
              <span className="text-bg-500">
                Auto refresh: <span className="text-bg-800 font-medium">{activeTab === 'dns' ? '5' : '∞'}s</span>
              </span>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-6xl mx-auto px-6 py-8">
        {/* Tab Navigation */}
        <div className="border-b border-bg-200 mb-6">
          <ul className="flex space-x-8">
            <li>
              <button
                onClick={() => switchTab('dns')}
                className={`py-4 px-1 border-b-2 font-medium transition-colors ${
                  activeTab === 'dns'
                    ? 'border-accent-500 text-accent-600'
                    : 'border-transparent text-bg-500 hover:text-bg-700 hover:border-bg-300'
                }`}
              >
                DNS Monitor
              </button>
            </li>
            <li>
              <button
                onClick={() => switchTab('mitm')}
                className={`py-4 px-1 border-b-2 font-medium transition-colors ${
                  activeTab === 'mitm'
                    ? 'border-accent-500 text-accent-600'
                    : 'border-transparent text-bg-500 hover:text-bg-700 hover:border-bg-300'
                }`}
              >
                MITM Traffic
              </button>
            </li>
            <li>
              <button
                onClick={() => switchTab('conversations')}
                className={`py-4 px-1 border-b-2 font-medium transition-colors ${
                  activeTab === 'conversations'
                    ? 'border-accent-500 text-accent-600'
                    : 'border-transparent text-bg-500 hover:text-bg-700 hover:border-bg-300'
                }`}
              >
                LLM Conversations
              </button>
            </li>
          </ul>
        </div>

        {/* Tab Content */}
        {activeTab === 'dns' && <DnsMonitor />}
        {activeTab === 'mitm' && <MitmTraffic />}
        {activeTab === 'conversations' && <Conversations />}
      </main>

      {/* Footer */}
      <footer className="border-t border-bg-200 mt-8">
        <div className="max-w-6xl mx-auto px-6 py-4">
          <div className="flex items-center justify-between text-xs text-bg-400">
            <span>Linko Monitor</span>
          </div>
        </div>
      </footer>
    </div>
  );
}

export default App;
