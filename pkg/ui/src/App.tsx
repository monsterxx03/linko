import { useState, useEffect } from 'react';
import MitmTraffic from './pages/MitmTraffic';
import Conversations from './pages/Conversations';
import { SSEProvider } from './contexts/SSEContext';

type Tab = 'mitm' | 'conversations';

const TAB_VALUES: Tab[] = ['mitm', 'conversations'];

function getTabFromHash(): Tab {
  const hash = window.location.hash.slice(1);
  return TAB_VALUES.includes(hash as Tab) ? (hash as Tab) : 'mitm';
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
    <SSEProvider>
      <div className="min-h-screen bg-bg-50">
      {/* Header */}
      <header className="bg-white border-b border-bg-200 sticky top-0 z-10">
        <div className="w-full px-6 py-4">
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

            {/* Tab Navigation - Moved to header for sticky positioning */}
            <div className="flex items-center gap-4">
              <ul className="flex items-center gap-1 p-1 bg-bg-100 rounded-lg">
                <li>
                  <button
                    onClick={() => switchTab('mitm')}
                    className={`px-4 py-1.5 text-sm font-medium rounded-md transition-colors ${
                      activeTab === 'mitm'
                        ? 'bg-white text-accent-600 shadow-sm'
                        : 'text-bg-600 hover:text-bg-800'
                    }`}
                  >
                    MITM
                  </button>
                </li>
                <li>
                  <button
                    onClick={() => switchTab('conversations')}
                    className={`px-4 py-1.5 text-sm font-medium rounded-md transition-colors ${
                      activeTab === 'conversations'
                        ? 'bg-white text-accent-600 shadow-sm'
                        : 'text-bg-600 hover:text-bg-800'
                    }`}
                  >
                    LLM
                  </button>
                </li>
              </ul>
              <span className="w-px h-6 bg-bg-200" />
              <div className="flex items-center gap-2 text-sm text-bg-600">
                <div className="w-2 h-2 rounded-full bg-emerald-500" />
                <span>Connected</span>
              </div>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="w-full px-6 py-8 min-h-0">
        {/* Tab Content - Use visibility hidden instead of display none to preserve scroll position */}
        <div className="contents">
          <div className={activeTab === 'mitm' ? '' : 'invisible absolute w-full pointer-events-none'}>
            <MitmTraffic />
          </div>
          <div className={activeTab === 'conversations' ? '' : 'invisible absolute w-full pointer-events-none'}>
            <Conversations />
          </div>
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-bg-200 mt-8">
        <div className="w-full px-6 py-4">
          <div className="flex items-center justify-between text-xs text-bg-400">
            <span>Linko Monitor</span>
          </div>
        </div>
      </footer>
    </div>
    </SSEProvider>
  );
}

export default App;
